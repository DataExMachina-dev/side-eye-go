package snapshot

import (
	"unsafe"

	"github.com/DataExMachina-dev/side-eye-go/internal/framing"
	"github.com/DataExMachina-dev/side-eye-go/internal/snapshotpb"
	. "github.com/DataExMachina-dev/side-eye-go/internal/stackmachine"
)

// TODO: Figure out if these generics buy literally anything compared to just
// using a interfaces. Also worth comparing against the concrete types.
type stackMachine struct {
	stack []uint32

	// Pointer to the end of the outBuf
	offset  uint32
	cfa     uintptr
	decoder OpDecoder

	frameOffset uint32
	frameHeader *framing.FrameHeader

	goContextOffset         uint32
	goContextCaptureBitmask uint64

	q *queue
	b *outBuf
	g *goRuntimeTypeResolver
	t *typeIdResolver
	p *snapshotpb.SnapshotProgram
}

type Queue interface {
	Push(addr uintptr, t uint32, len uint32) bool
}

// OutBuf is an interface for interacting with the output buffer.
type OutBuf interface {
	// Ptr returns a pointer to the memory at the given offset.
	Ptr(offset uint32) unsafe.Pointer
	// PrepareFrameData writes the frame header and queue entry to the outBuf and returns
	// the offset to the data location for the queue entry.
	PrepareFrameData(typeID uint32, progID uint32, dataLen uint32, depth uint32) (offset uint32, ok bool)
	// GetEntryLen assumes that the passed offset is immediately following a queue entry
	// and it extracts the length from that queue entry.
	GetEntryLen(entryOffset uint32) uint32

	// Zero the memory at the given offset for the given length.
	Zero(offset uint32, len uint32)

	// Dereference the given memory range.
	Dereference(offset uint32, ptr uintptr, len uint32) bool

	// The length of the outBuf in bytes.
	Len() uint32
}

type GoRuntimeTypeResolver interface {
	ResolveTypeAddressToGoRuntimeTypeId(addr uint64) uint64
}

type TypeIDResolver interface {
	ResolveGoRuntimeTypeToTypeId(addr uint64) uint32
}

func newStackMachine(
	p *snapshotpb.SnapshotProgram,
	q *queue,
	b *outBuf,
	g *goRuntimeTypeResolver,
	t *typeIdResolver,
) *stackMachine {
	return &stackMachine{
		stack:   make([]uint32, 0, 64),
		decoder: MakeOpDecoder(p.Prog),
		q:       q,
		b:       b,
		g:       g,
		t:       t,
		p:       p,
	}
}

type ResolvedEmptyInterface struct {
	addr          uintptr
	goRuntimeType uint64
}

func (s *stackMachine) resolveEmptyInterface() *ResolvedEmptyInterface {
	ptr := s.b.Ptr(s.offset)
	type runtimeEface struct {
		_type uintptr
		data  uintptr
	}
	var e runtimeEface
	e = *(*runtimeEface)(ptr)
	// nil eface
	if e._type == 0 {
		return nil
	}
	goRuntimeType := s.g.ResolveTypeAddressToGoRuntimeTypeId((uint64(e._type)))
	return &ResolvedEmptyInterface{
		addr:          e.data,
		goRuntimeType: goRuntimeType,
	}
}

type ResolvedAnyType struct {
	e        ResolvedEmptyInterface
	typeId   uint32
	typeInfo *snapshotpb.TypeInfo
}

func (s *stackMachine) resolveAnyType() *ResolvedAnyType {
	e := s.resolveEmptyInterface()
	if e == nil || e.goRuntimeType == 0 {
		return nil
	}
	typeId := s.t.ResolveGoRuntimeTypeToTypeId(e.goRuntimeType)
	if typeId == 0 {
		return nil
	}
	ti, ok := s.p.TypeInfo[typeId]
	if !ok {
		return nil
	}
	return &ResolvedAnyType{
		e:        *e,
		typeId:   typeId,
		typeInfo: ti,
	}
}

func (s *stackMachine) recordGoContextValue(spec *snapshotpb.GoContextValueType,
	value *ResolvedAnyType, expectedType uint32) {
	// Check if this value is already captured
	if s.goContextCaptureBitmask&(uint64(1)<<uint64(spec.Index)) == 0 {
		return
	}
	s.goContextCaptureBitmask &= ^(uint64(1) << uint64(spec.Index))

	// Record the reference to the value.
	ptr := s.b.Ptr(s.goContextOffset + spec.Offset + 0)
	*(*uintptr)(ptr) = value.e.addr
	ptr = s.b.Ptr(s.goContextOffset + spec.Offset + 8)
	*(*uint64)(ptr) = value.e.goRuntimeType

	if expectedType != 0 && expectedType != value.typeId {
		// Type mismatch, just bail, recorded reference will expose the issue upstream.
		return
	}

	// Queue the value
	t := spec.Type
	if t == 0 {
		t = value.typeId
	}
	s.q.Push(value.e.addr, t, 0)
}

func (s *stackMachine) Run(
	pc uint32,
	cfa uintptr,
	depth uint32,
	offset uint32,
) bool {
	if !s.decoder.SetPC(pc) {
		return false
	}
	s.cfa = cfa
	s.offset = offset

	for i := 0; i < 100000; i++ {
		op := s.decoder.PopOpCode()
		switch op {
		case OpCodeInvalid:
			return false
		case OpCodeCall:
			call := s.decoder.DecodeCall()
			s.stack = append(s.stack, s.decoder.PC())
			if !s.decoder.SetPC(call.Pc) {
				return false
			}

		case OpCodeCondJump:
			condJump := s.decoder.DecodeCondJump()
			if s.stack[len(s.stack)-1] != 0 {
				if !s.decoder.SetPC(condJump.Pc) {
					return false
				}
			}

		case OpCodeDecrement:
			_ = s.decoder.DecodeDecrement()
			s.stack[len(s.stack)-1] -= 1

		case OpCodeEnqueueEmptyInterface:
			_ = s.decoder.DecodeEnqueueEmptyInterface()
			e := s.resolveEmptyInterface()
			if e == nil {
				continue
			}
			ptr := s.b.Ptr(s.offset)
			*(*uint64)(ptr) = e.goRuntimeType
			typeId := s.t.ResolveGoRuntimeTypeToTypeId(e.goRuntimeType)
			if typeId == 0 {
				continue
			}
			s.q.Push(e.addr, typeId, 0)

		case OpCodeEnqueueInterface:
			_ = s.decoder.DecodeEnqueueEmptyInterface()
			ptr := s.b.Ptr(s.offset)
			type runtimeIface struct {
				itab uintptr
				data uintptr
			}
			e := (*runtimeIface)(ptr)
			// nil iface
			if e.itab == 0 {
				continue
			}
			_typeAddr := unsafe.Pointer(uintptr(e.itab) + 8)
			if !s.b.Dereference(s.offset, uintptr(_typeAddr), 8) {
				continue
			}
			goRuntimeType := s.g.ResolveTypeAddressToGoRuntimeTypeId(uint64(e.itab))
			*(*uint64)(ptr) = goRuntimeType
			typeId := s.t.ResolveGoRuntimeTypeToTypeId(goRuntimeType)
			if typeId == 0 {
				continue
			}
			s.q.Push(e.data, typeId, 0)

		case OpCodeEnqueuePointer:
			enqueuePointer := s.decoder.DecodeEnqueuePointer()
			if enqueuePointer.ElemType == 0 {
				return false
			}

			addr := *(*uintptr)(s.b.Ptr(s.offset))
			s.q.Push(addr, enqueuePointer.ElemType, 0)

		case OpCodeEnqueueBiasedPointer:
			enqueuePointer := s.decoder.DecodeEnqueueBiasedPointer()
			if enqueuePointer.ElemType == 0 {
				return false
			}
			addr := *(*uintptr)(s.b.Ptr(s.offset)) +
				uintptr(enqueuePointer.Bias)
			s.q.Push(addr, enqueuePointer.ElemType, 0)

		case OpCodeEnqueueSliceHeader:
			enqueueSliceHeader := s.decoder.DecodeEnqueueSliceHeader()
			// TODO: Replace this offset with something in the bytecode or some other config
			// that is not hard-coded.
			len := *(*int)(s.b.Ptr(s.offset + 8))
			if len > 0 && enqueueSliceHeader.ElemByteLen > 0 {
				addr := *(*uintptr)(s.b.Ptr(s.offset))
				s.q.Push(
					addr,
					enqueueSliceHeader.ArrayType,
					uint32(len*int(enqueueSliceHeader.ElemByteLen)),
				)
			}

		case OpCodeEnqueueStringHeader:
			enqueueStringHeader := s.decoder.DecodeEnqueueStringHeader()
			// TODO: Replace this offset with something in the bytecode or some other config
			// that is not hard-coded.
			len := *(*int)(s.b.Ptr(s.offset + 8))
			if len > 0 {
				addr := *(*uintptr)(s.b.Ptr(s.offset))
				s.q.Push(addr, enqueueStringHeader.StringDataType, uint32(len))
			}

		case OpCodeEnqueueHMapHeader:
			// We enqueue new and old buckets at once, no need to loop (unlike ebpf probe).
			s.stack[len(s.stack)-1] = 0

			enqueueMapHeader := s.decoder.DecodeEnqueueHMapHeader()
			// https://github.com/golang/go/blob/8d04110c/src/runtime/map.go#L105
			const sameSizeGrow uint8 = 8
			flags := *(*uint8)(s.b.Ptr(s.offset + uint32(enqueueMapHeader.FlagsOffset)))
			b := *(*uint8)(s.b.Ptr(s.offset + uint32(enqueueMapHeader.BOffset)))

			bucketsAddr := *(*uintptr)(s.b.Ptr(s.offset + uint32(enqueueMapHeader.BucketsOffset)))
			if bucketsAddr != 0 {
				numBuckets := uint32(1 << b)
				bucketsSize := numBuckets * uint32(enqueueMapHeader.BucketByteLen)
				s.q.Push(bucketsAddr, enqueueMapHeader.BucketsArrayType, bucketsSize)
			}
			oldBucketsAddr := *(*uintptr)(s.b.Ptr(s.offset + uint32(enqueueMapHeader.OldBucketsOffset)))
			if oldBucketsAddr != 0 {
				numBuckets := uint32(1 << b)
				if (flags & sameSizeGrow) == 0 {
					numBuckets >>= 1
				}
				oldBucketsSize := numBuckets * uint32(enqueueMapHeader.BucketByteLen)
				s.q.Push(oldBucketsAddr, enqueueMapHeader.BucketsArrayType, oldBucketsSize)
			}

		case OpCodeEnqueueSwissMap:
			p := s.decoder.DecodeEnqueueSwissMap()
			dirPtr := *(*uintptr)(s.b.Ptr(s.offset + uint32(p.DirPtrOffset)))
			dirLen := *(*int64)(s.b.Ptr(s.offset + uint32(p.DirLenOffset)))
			if dirLen > 0 {
				s.q.Push(dirPtr, p.TablePtrSliceType, uint32(8*dirLen))
			} else {
				s.q.Push(dirPtr, p.GroupType, 0)
			}

		case OpCodeEnqueueSwissMapGroups:
			p := s.decoder.DecodeEnqueueSwissMapGroups()
			data := *(*uintptr)(s.b.Ptr(s.offset + uint32(p.DataOffset)))
			lengthMask := *(*uint64)(s.b.Ptr(s.offset + uint32(p.LengthMaskOffset)))
			s.q.Push(data, p.GroupSliceType, p.GroupByteLen*(uint32(lengthMask)+1))

		case OpCodeJump:
			jump := s.decoder.DecodeJump()
			if !s.decoder.SetPC(jump.Pc) {
				return false
			}

		case OpCodePop:
			_ = s.decoder.DecodePop()
			s.stack = s.stack[:len(s.stack)-1]

		case OpCodePushImm:
			pushImm := s.decoder.DecodePushImm()
			s.stack = append(s.stack, pushImm.Value)

		case OpCodePushOffset:
			pushOffset := s.decoder.DecodePushOffset()
			_ = pushOffset
			s.stack = append(s.stack, s.offset)

		case OpCodePushSliceLen:
			pushSliceLen := s.decoder.DecodePushSliceLen()
			entryLen := s.b.GetEntryLen(s.offset)
			s.stack = append(s.stack, entryLen/uint32(pushSliceLen.ElemByteLen))

		case OpCodeReturn:
			if len(s.stack) == 0 {
				return true
			}
			if !s.decoder.SetPC(s.stack[len(s.stack)-1]) {
				return false
			}
			s.stack[len(s.stack)-1] = 0 // not needed
			s.stack = s.stack[:len(s.stack)-1]

		case OpCodeSetOffset:
			setOffset := s.decoder.DecodeSetOffset()
			_ = setOffset
			s.offset = s.stack[len(s.stack)-1]

		case OpCodeShiftOffset:
			shiftOffset := s.decoder.DecodeShiftOffset()
			s.offset += shiftOffset.Increment

		case OpCodeDereferenceCFAOffset:
			dereferenceCFAOffset := s.decoder.DecodeDereferenceCFAOffset()
			srcAddr := uintptr(s.cfa) +
				uintptr(dereferenceCFAOffset.Offset) +
				uintptr(dereferenceCFAOffset.PointerBias)
			s.b.Dereference(s.offset, srcAddr, dereferenceCFAOffset.ByteLen)

		case OpCodeCopyFromRegister:
			copyFromRegister := s.decoder.DecodeCopyFromRegister()
			_ = copyFromRegister
			s.b.Zero(s.offset, 8)

		case OpCodeZeroFill:
			zeroFill := s.decoder.DecodeZeroFill()
			s.b.Zero(s.offset, zeroFill.ByteLen)

		case OpCodeSetPresenceBit:
			setPresenceBit := s.decoder.DecodeSetPresenceBit()
			ptr := s.b.Ptr(s.frameOffset + setPresenceBit.BitOffset/8)
			*(*uint8)(ptr) |= (1 << (setPresenceBit.BitOffset % 8))

		case OpCodePrepareFrameData:
			prepareFrameData := s.decoder.DecodePrepareFrameData()
			frameHeader, offset, ok := s.b.PrepareFrameData(
				prepareFrameData.TypeID,
				prepareFrameData.ProgID,
				prepareFrameData.DataByteLen,
				depth,
			)
			if !ok {
				// TODO: handle this error
				return false
			}
			s.offset = offset
			s.frameOffset = offset
			s.frameHeader = frameHeader

		case OpCodeConcludeFrameData:
			s.b.ConcludeFrameData(s.frameHeader)

		case OpCodePrepareGoContext:
			c := s.decoder.DecodePrepareGoContext()
			type runtimeIface struct {
				itab uintptr
				data uintptr
			}
			e := (*runtimeIface)(s.b.Ptr(s.offset))
			// We use the address of the first object behind the interface
			// as the address of the captured synthetic go context object.
			if !s.q.ShouldRecord(e.data, c.TypeID) {
				continue
			}
			o, ok := s.b.writeQueueEntry(framing.QueueEntry{
				Addr: uint64(e.data),
				Type: c.TypeID,
				Len:  c.DataByteLen,
			})
			if !ok {
				continue
			}
			s.b.Zero(o, c.DataByteLen)
			s.goContextOffset = o
			s.goContextCaptureBitmask = (uint64(1) << uint64(c.CaptureCount)) - 1
			// We will need to fetch some data into the buf, that we will
			// later truncate.
			truncateTarget := s.b.Len()
			// Iterate over go context implementation stack.
			for {
				if s.goContextCaptureBitmask == 0 {
					break
				}
				e = (*runtimeIface)(s.b.Ptr(s.offset))
				if e.itab == 0 {
					break
				}
				_typeAddr := unsafe.Pointer(uintptr(e.itab) + 8)
				if !s.b.Dereference(s.offset, uintptr(_typeAddr), 8) {
					break
				}
				goRuntimeType := s.g.ResolveTypeAddressToGoRuntimeTypeId(uint64(e.itab))
				typeId := s.t.ResolveGoRuntimeTypeToTypeId(goRuntimeType)
				if typeId == 0 {
					break
				}
				cti, ok := s.p.TypeInfo[typeId]
				if !ok {
					break
				}
				if cti.GoContextImpl == nil {
					break
				}
				s.offset, ok = s.b.writeQueueEntry(framing.QueueEntry{
					Addr: uint64(e.data),
					Type: typeId,
					Len:  cti.ByteLen,
				})
				if !ok {
					break
				}
				// Check if there is there is an interesting value
				if cti.GoContextImpl.ValueOffset != nil {
					s.offset += *cti.GoContextImpl.ValueOffset
					v := s.resolveAnyType()
					s.offset -= *cti.GoContextImpl.ValueOffset
					if v != nil {
						if v.typeInfo.GoContextValue != nil {
							s.recordGoContextValue(v.typeInfo.GoContextValue, v, 0)
						}
						if cti.GoContextImpl.KeyOffset != nil {
							s.offset += *cti.GoContextImpl.KeyOffset
							k := s.resolveAnyType()
							s.offset -= *cti.GoContextImpl.KeyOffset
							if k != nil && k.typeInfo.GoContextKey != nil {
								s.recordGoContextValue(
									k.typeInfo.GoContextKey, v, *k.typeInfo.GoContextKeyValueType)
							}
						}
					}
				}

				// Check if there is another wrapped context.
				if cti.GoContextImpl.ContextOffset == nil {
					break
				}
				s.offset += *cti.GoContextImpl.ContextOffset
			}
			s.b.truncate(truncateTarget)

		case OpCodeTraverseGoContext:
		case OpCodeConcludeGoContext:
			// Not used in the go stack machine.

		case OpCodeIllegal:
			// This should be totally bogus and generally will not be aligned.
			v := (*uint64)(unsafe.Pointer(uintptr(pc)))
			if *v > 0 {
				return false
			}

		}
	}

	return false
}
