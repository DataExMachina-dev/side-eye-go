package stackmachine

import (
	"encoding/binary"
	"fmt"
)

// OpDecoder is a decoder for stack machine operations.
type OpDecoder struct {
	pc    uint32
	opBuf []byte
}

// MakeOpDecoder creates a new OpDecoder.
func MakeOpDecoder(opBuf []byte) OpDecoder {
	return OpDecoder{
		pc:    0,
		opBuf: opBuf,
	}
}

// SetPC sets the program counter of the decoder.
func (d *OpDecoder) SetPC(pc uint32) bool {
	if pc >= uint32(len(d.opBuf)) {
		return false
	}
	d.pc = pc
	return true
}

// PC returns the program counter of the decoder.
func (d *OpDecoder) PC() uint32 {
	return d.pc
}

type OpCode uint8

//go:generate go run golang.org/x/tools/cmd/stringer@v0.22.0 -type=OpCode
const (
	OpCodeInvalid               OpCode = 0
	OpCodeChasePointers         OpCode = 26
	OpCodeCall                  OpCode = 1
	OpCodeCondJump              OpCode = 2
	OpCodeDecrement             OpCode = 3
	OpCodeEnqueueEmptyInterface OpCode = 4
	OpCodeEnqueueInterface      OpCode = 5
	OpCodeEnqueuePointer        OpCode = 6
	OpCodeEnqueueSliceHeader    OpCode = 7
	OpCodeEnqueueStringHeader   OpCode = 8
	OpCodeEnqueueHMapHeader     OpCode = 9
	OpCodeEnqueueSwissMap       OpCode = 31
	OpCodeEnqueueSwissMapGroups OpCode = 32
	OpCodeEnqueueSubroutine     OpCode = 33
	OpCodeJump                  OpCode = 10
	OpCodePop                   OpCode = 11
	OpCodePushImm               OpCode = 12
	OpCodePushOffset            OpCode = 13
	OpCodePushSliceLen          OpCode = 14
	OpCodeReturn                OpCode = 15
	OpCodeSetOffset             OpCode = 16
	OpCodeAdvanceOffset         OpCode = 17
	OpCodeDereferenceCFAOffset  OpCode = 19
	OpCodeCopyFromRegister      OpCode = 20
	OpCodePrepareExprEval       OpCode = 36
	OpCodeSaveFrameResult       OpCode = 34
	OpCodeDereferencePtr        OpCode = 35
	OpCodeZeroFill              OpCode = 21
	OpCodeSetPresenceBit        OpCode = 30
	OpCodePrepareFrameData      OpCode = 22
	OpCodeConcludeFrameData     OpCode = 25
	PrepareEventData            OpCode = 24
	OpCodePrepareGoContext      OpCode = 27
	OpCodeTraverseGoContext     OpCode = 28
	OpCodeConcludeGoContext     OpCode = 29
	OpCodeIllegal               OpCode = 23
)

type (
	OpCall struct {
		Pc uint32
	}
	OpCondJump struct {
		Pc uint32
	}
	OpDecrement             struct{}
	OpEnqueueEmptyInterface struct{}
	OpEnqueueInterface      struct{}
	OpEnqueuePointer        struct {
		ElemType uint32
	}
	OpEnqueueSliceHeader struct {
		ArrayType   uint32
		ElemByteLen uint32
	}
	OpEnqueueStringHeader struct {
		StringDataType uint32
	}
	OpEnqueueHMapHeader struct {
		BucketsArrayType uint32
		BucketByteLen    uint32
		FlagsOffset      uint8
		BOffset          uint8
		BucketsOffset    uint8
		OldBucketsOffset uint8
	}
	OpEnqueueSwissMap struct {
		TablePtrSliceType uint32
		GroupType         uint32
		DirPtrOffset      uint8
		DirLenOffset      uint8
	}
	OpEnqueueSwissMapGroups struct {
		GroupSliceType   uint32
		GroupByteLen     uint32
		DataOffset       uint8
		LengthMaskOffset uint8
	}
	OpEnqueueSubroutine struct{}
	OpJump              struct {
		Pc uint32
	}
	OpPop     struct{}
	OpPushImm struct {
		Value uint32
	}
	OpPushOffset   struct{}
	OpPushSliceLen struct {
		ElemByteLen uint32
	}
	OpReturn        struct{}
	OpSetOffset     struct{}
	OpAdvanceOffset struct {
		Increment uint32
	}
	OpDereferenceCFAOffset struct {
		Offset      int32
		ByteLen     uint32
		PointerBias uint32
	}
	OpCopyFromRegister struct {
		Register uint16
		ByteSize uint8
	}
	OpSaveFrameResult struct {
		FrameOffset uint32
		ByteLen     uint32
	}
	OpDereferencePtr struct {
		Bias    uint32
		ByteLen uint32
	}
	OpZeroFill struct {
		ByteLen uint32
	}
	OpSetPresenceBit struct {
		BitOffset uint32
	}
	OpPrepareFrameData struct {
		ProgID      uint32
		DataByteLen uint32
		TypeID      uint32
	}
	OpPrepareGoContext struct {
		DataByteLen  uint32
		TypeID       uint32
		CaptureCount uint8
	}
	OpTraverseGoContext struct{}
	OpConcludeGoContext struct{}
	OpIllegal           struct{}
)

func (d *OpDecoder) PopOpCode() OpCode {
	code := OpCode(d.opBuf[d.pc])
	d.pc += 1
	return code
}

func (d *OpDecoder) DecodeCall() OpCall {
	pc := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpCall{
		Pc: pc,
	}
}

func (d *OpDecoder) DecodeCondJump() OpCondJump {
	pc := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpCondJump{
		Pc: pc,
	}
}
func (d *OpDecoder) DecodeDecrement() OpDecrement {
	return OpDecrement{}
}
func (d *OpDecoder) DecodeEnqueueEmptyInterface() OpEnqueueEmptyInterface {
	return OpEnqueueEmptyInterface{}
}
func (d *OpDecoder) DecodeEnqueueInterface() OpEnqueueInterface {
	return OpEnqueueInterface{}
}
func (d *OpDecoder) DecodeEnqueuePointer() OpEnqueuePointer {
	elemType := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpEnqueuePointer{
		ElemType: elemType,
	}
}
func (d *OpDecoder) DecodeEnqueueSliceHeader() OpEnqueueSliceHeader {
	arrayType := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	elemByteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc+4:])
	d.pc += 8
	return OpEnqueueSliceHeader{
		ArrayType:   arrayType,
		ElemByteLen: elemByteLen,
	}
}
func (d *OpDecoder) DecodeEnqueueStringHeader() OpEnqueueStringHeader {
	stringDataType := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpEnqueueStringHeader{
		StringDataType: stringDataType,
	}
}
func (d *OpDecoder) DecodeEnqueueHMapHeader() OpEnqueueHMapHeader {
	op := OpEnqueueHMapHeader{
		BucketsArrayType: binary.LittleEndian.Uint32(d.opBuf[d.pc:]),
		BucketByteLen:    binary.LittleEndian.Uint32(d.opBuf[d.pc+4:]),
		FlagsOffset:      d.opBuf[d.pc+8],
		BOffset:          d.opBuf[d.pc+9],
		BucketsOffset:    d.opBuf[d.pc+10],
		OldBucketsOffset: d.opBuf[d.pc+11],
	}
	d.pc += 12
	return op
}
func (d *OpDecoder) DecodeEnqueueSwissMap() OpEnqueueSwissMap {
	op := OpEnqueueSwissMap{
		TablePtrSliceType: binary.LittleEndian.Uint32(d.opBuf[d.pc:]),
		GroupType:         binary.LittleEndian.Uint32(d.opBuf[d.pc+4:]),
		DirPtrOffset:      d.opBuf[d.pc+8],
		DirLenOffset:      d.opBuf[d.pc+9],
	}
	d.pc += 10
	return op
}
func (d *OpDecoder) DecodeEnqueueSwissMapGroups() OpEnqueueSwissMapGroups {
	op := OpEnqueueSwissMapGroups{
		GroupSliceType:   binary.LittleEndian.Uint32(d.opBuf[d.pc:]),
		GroupByteLen:     binary.LittleEndian.Uint32(d.opBuf[d.pc+4:]),
		DataOffset:       d.opBuf[d.pc+8],
		LengthMaskOffset: d.opBuf[d.pc+9],
	}
	d.pc += 10
	return op
}
func (d *OpDecoder) DecodeEnqueueSubroutine() OpEnqueueSubroutine {
	return OpEnqueueSubroutine{}
}
func (d *OpDecoder) DecodeJump() OpJump {
	pc := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpJump{
		Pc: pc,
	}
}
func (d *OpDecoder) DecodePop() OpPop {
	return OpPop{}
}
func (d *OpDecoder) DecodePushImm() OpPushImm {
	value := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpPushImm{
		Value: value,
	}
}
func (d *OpDecoder) DecodePushOffset() OpPushOffset {
	return OpPushOffset{}
}
func (d *OpDecoder) DecodePushSliceLen() OpPushSliceLen {
	elemByteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpPushSliceLen{
		ElemByteLen: elemByteLen,
	}
}
func (d *OpDecoder) DecodeReturn() OpReturn {
	return OpReturn{}
}
func (d *OpDecoder) DecodeSetOffset() OpSetOffset {
	return OpSetOffset{}
}
func (d *OpDecoder) DecodeAdvanceOffset() OpAdvanceOffset {
	increment := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpAdvanceOffset{
		Increment: increment,
	}
}
func (d *OpDecoder) DecodeDereferenceCFAOffset() OpDereferenceCFAOffset {
	offset := int32(binary.LittleEndian.Uint32(d.opBuf[d.pc:]))
	byteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc+4:])
	pointerBias := binary.LittleEndian.Uint32(d.opBuf[d.pc+8:])
	d.pc += 12
	return OpDereferenceCFAOffset{
		Offset:      offset,
		ByteLen:     byteLen,
		PointerBias: pointerBias,
	}
}
func (d *OpDecoder) DecodeCopyFromRegister() OpCopyFromRegister {
	register := binary.LittleEndian.Uint16(d.opBuf[d.pc:])
	byteSize := uint8(d.opBuf[d.pc+2])
	d.pc += 3
	return OpCopyFromRegister{
		Register: register,
		ByteSize: byteSize,
	}
}
func (d *OpDecoder) DecodeSaveFrameResult() OpSaveFrameResult {
	frameOffset := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	byteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc+4:])
	d.pc += 8
	return OpSaveFrameResult{
		FrameOffset: frameOffset,
		ByteLen:     byteLen,
	}
}
func (d *OpDecoder) DecodeDereferencePtr() OpDereferencePtr {
	bias := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	byteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc+4:])
	d.pc += 8
	return OpDereferencePtr{
		Bias:    bias,
		ByteLen: byteLen,
	}
}
func (d *OpDecoder) DecodeZeroFill() OpZeroFill {
	byteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpZeroFill{
		ByteLen: byteLen,
	}
}
func (d *OpDecoder) DecodeSetPresenceBit() OpSetPresenceBit {
	byteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	d.pc += 4
	return OpSetPresenceBit{
		BitOffset: byteLen,
	}
}
func (d *OpDecoder) DecodePrepareFrameData() OpPrepareFrameData {
	progID := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	dataByteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc+4:])
	typeID := binary.LittleEndian.Uint32(d.opBuf[d.pc+8:])
	d.pc += 12
	return OpPrepareFrameData{
		ProgID:      progID,
		DataByteLen: dataByteLen,
		TypeID:      typeID,
	}
}
func (d *OpDecoder) DecodePrepareGoContext() OpPrepareGoContext {
	dataByteLen := binary.LittleEndian.Uint32(d.opBuf[d.pc:])
	typeID := binary.LittleEndian.Uint32(d.opBuf[d.pc+4:])
	captureCount := uint8(d.opBuf[d.pc+8:][0])
	d.pc += 9
	return OpPrepareGoContext{
		DataByteLen:  dataByteLen,
		TypeID:       typeID,
		CaptureCount: captureCount,
	}
}
func (d *OpDecoder) DecodeTraverseGoContext() OpTraverseGoContext {
	return OpTraverseGoContext{}
}
func (d *OpDecoder) DecodeConcludeGoContext() OpConcludeGoContext {
	return OpConcludeGoContext{}
}
func (d *OpDecoder) DecodeIllegal() OpIllegal {
	return OpIllegal{}
}

type Op struct {
	Pc   int32
	Code OpCode
	Op   any
}

func (d *Op) String() string {
	return fmt.Sprintf("Op{Pc: %d, Code: %s, Op: %v}", d.Pc, d.Code, d.Op)
}
