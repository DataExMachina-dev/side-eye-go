package snapshot

import (
	"unsafe"

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

	q *queue
	b *outBuf
	g *goRuntimeTypeResolver
	t *typeIdResolver
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
	prog []byte,
	q *queue,
	b *outBuf,
	g *goRuntimeTypeResolver,
	t *typeIdResolver,
) *stackMachine {
	return &stackMachine{
		stack:   make([]uint32, 0, 64),
		decoder: MakeOpDecoder(prog),
		q:       q,
		b:       b,
		g:       g,
		t:       t,
	}
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
			ptr := s.b.Ptr(s.offset)
			type runtimeEface struct {
				_type uintptr
				data  uintptr
			}
			var e runtimeEface
			e = *(*runtimeEface)(ptr)
			// nil eface
			if e._type == 0 {
				continue
			}
			goRuntimeType := s.g.ResolveTypeAddressToGoRuntimeTypeId(uint64(e._type))
			*(*uint64)(ptr) = goRuntimeType
			typeId := s.t.ResolveGoRuntimeTypeToTypeId(goRuntimeType)
			if typeId == 0 {
				continue
			}
			s.q.Push(e.data, typeId, 0)

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

		case OpCodeEnqueueMapHeader:
			// We enqueue new and old buckets at once, no need to loop (unlike ebpf probe).
			s.stack[len(s.stack)-1] = 0

			enqueueMapHeader := s.decoder.DecodeEnqueueMapHeader()
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

		case OpCodePrepareFrameData:
			prepareFrameData := s.decoder.DecodePrepareFrameData()
			offset, ok := s.b.PrepareFrameData(
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
