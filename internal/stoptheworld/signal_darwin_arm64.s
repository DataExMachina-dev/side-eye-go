// Copyright 2009 The Go Authors. All rights reserved.
// Loosely derived from work in https://github.com/golang/go/blob/9a9dd72d/src/runtime/sys_linux_arm64.s

#include "go_asm.h"
#include "textflag.h"
#include "abi_arm64.h"

#define SIGBUS 10
#define SIGSEGV 11

TEXT notok<>(SB),NOSPLIT,$0
	MOVD	$0, R8
	MOVD	R8, (R8)
	B	0(PC)

// setHandler sets up the sigsegvHandler and stores the old handler in oldHandler.
//
// func setHandler() {
//     state.snapshotTid = gettid()
//     var sa sigactiont
//     { // initialize_sa
//         sa.sa_flags = saFlags
//         sa.sa_restorer = runtime·sigreturn__sigaction
//         sa.handler = sigsegvHandler
//         sa.sa_mask = ^uint64(0)
//     }
//     return rt_sigaction(SIGSEGV, &sa, &state.prevAction, unsafe.Sizeof(sa.sa_mask))
// }
TEXT ·setHandler(SB), NOSPLIT, $0
    // var sa usigactiont
    SUB     $0x20, RSP
    MOVD    $SIGSEGV, R3
start:
    MOVD    $0, R0                        // Zero out R0
    MOVD    $usigactiont__size, R1        // Load the size of sigactiont
    MOVD    RSP, R2                       // R2 points to the start of sigactiont

clear_loop:
    CMP     $0, R1                        // Check if R1 is zero
    BEQ     initialize_sa                 // If zero, exit loop
    MOVD    $0, (R2)
    ADD     $8, R2                       // Move to the next word.
    SUB     $8, R1                       // Decrease R1 by 8
    B       clear_loop                   // Repeat loop

initialize_sa:
    // sa.sa_flags = saFlags
    MOVW    $const_saFlags, R2
    MOVW    R2, usigactiont_sa_flags(RSP)
    // sa.handler = sigsegvHandler
    MOVD    $·sigsegvHandler(SB), R0
    MOVD    R0, usigactiont___sigaction_u(RSP)
    // sa.sa_mask = ^uint64(0)
    MOVD    $0, R0
    MVN     R0, R0
    MOVW    R0, usigactiont_sa_mask(RSP)

    // return rt_sigaction(SIGSEGV, &sa, &state.prevAction, unsafe.Sizeof(sa.sa_mask))
    CMP     $SIGBUS, R3
    BEQ     sigbus
    MOVW    $SIGSEGV, R0
    MOVD    RSP, R1
    MOVD    $·state+signalState_prevAction+sigaction_prevSigsegv(SB), R2
    BL	    libc_sigaction(SB)
	CMP	    $0, R0
	BEQ	    2(PC)
	BL	    notok<>(SB)
    MOVD    $SIGBUS, R3
    B       start 
sigbus:
    MOVW    $SIGBUS, R0
    MOVD    RSP, R1
    MOVD    $·state+signalState_prevAction+sigaction_prevSigbus(SB), R2
    BL	    libc_sigaction(SB)
	CMP	    $0, R0
	BEQ	    2(PC)
	BL	    notok<>(SB)
done:
    ADD     $0x20, RSP
    RET

// resetHandler resets the handler to prevHandler.
//
// func resetHandler() {
//     state.snapshotTid = 0
//     return rt_sigaction(SIGSEGV, &state.prevAction, nil, unsafe.Sizeof(sa.sa_mask))
// }
//
TEXT ·resetHandler(SB), NOSPLIT, $0
    MOVD    $SIGSEGV, R0
    MOVD    $·state+signalState_prevAction+sigaction_prevSigsegv(SB), R1
    MOVD    $0, R2
    BL	libc_sigaction(SB)
	CMP	$0, R0
	BEQ	2(PC)
	BL	notok<>(SB)
    MOVD    $SIGBUS, R0
    MOVD    $·state+signalState_prevAction+sigaction_prevSigbus(SB), R1
    MOVD    $0, R2
    BL	libc_sigaction(SB)
	CMP	$0, R0
	BEQ	2(PC)
	BL	notok<>(SB)
    RET

// This is an arbitrary number of frames we need to check in order
// to find if we're in dereference. It probably only needs to be 3.
#define FRAMES_TO_CHECK 4

// A sigaction handler for segfaults that unwinds the stack a bit to look
// for a magic dereference function. If this function is found, then set
// the context to look like this function had returned 0. Otherwise, jump
// to the previously installed signal handler.
//
// func sigsegvHandler(sig uint64, info *siginfo, ctx *ucontext) {
//    tid := gettid()
//    if tid != state.snapshotTid {
//        goto passthrough
//    }
//    sigctx := ctx.uc_mcontext        // BX
//    pc := sigctx.rip                 // CX
//    fp := sigctx.rbp                 // R8
//    i := 0                           // SI
// loop_start:
//    if fp == 0 {
//        goto passthrough
//    }
//    next_fp = *(uintptr_t *)(fp)     // R9
//    next_pc = *(uintptr_t *)(fp + 8) // R10
//    if pc < state.dereferenceStart {
//        goto loop_continue
//    }
//    if pc >= state.dereferenceEnd {
//        goto loop_continue
//    }
//    sigctx.rbp = next_fp
//    sigctx.rip = next_pc
//    sigctx.rsp = fp + 16
//    sigctx.rax = 0 // mark failure
//    return
// loop_continue:
//    fp, pc = next_fp, next_pc
//    i += 1
//    if i < FRAMES_TO_CHECK {
//        goto loop_start
//    }
// passthrough:
//    exec(func() { state.prevAction.handler(sig, info, ctx) })
// }
TEXT ·sigsegvHandler(SB),NOSPLIT|TOPFRAME,$176
	// Save callee-save registers in the case of signal forwarding.
	// Please refer to https://golang.org/issue/31827 .
	SAVE_R19_TO_R28(8*4)
	SAVE_F8_TO_F15(8*14)

	// Save arguments.
	MOVW	R0, (8*1)(RSP)	// sig
	MOVD	R1, (8*2)(RSP)	// info
	MOVD	R2, (8*3)(RSP)	// ctx

    // sigctx := ctx.uc_mcontext        // R4
    MOVD    R2, R4 // BX = ctx
    MOVD    ucontext_uc_mcontext(R4), R4

    // pc := sigctx.rip                 // R5
    // fp := sigctx.rbp                 // R6
    MOVD    (mcontext64_ss + regs64_pc)(R4), R5 
    MOVD    (mcontext64_ss + regs64_fp)(R4), R6 

    // i := 0                           // R9
    MOVD    $0, R9

loop_start:
    // if fp == 0 {
    //     goto passthrough
    // }
    CBZ     R6, passthrough

    // next_pc = *(uintptr_t *)(fp + 8) // R7
    MOVD    8(R6), R7
    // next_fp = *(uintptr_t *)(fp)     // R8
    MOVD    (R6),  R8

    // if pc < state.dereferenceStart {
    //     goto loop_continue
    // }
    MOVD    ·state+signalState_dereferenceStart(SB), R0
    CMP     R0, R5
    BLT     loop_continue
    // if pc >= state.dereferenceEnd {
    //     goto loop_continue
    // }
    MOVD    ·state+signalState_dereferenceEnd(SB), R0
    CMP     R0, R5
    BGE     loop_continue

    // sigctx.rip = next_pc
    MOVD    R7, (mcontext64_ss + regs64_pc)(R4)
    // sigctx.lr = *next_fp+8
    MOVD    8(R8), R9
    MOVD    R9, (mcontext64_ss + regs64_lr)(R4)
    // sigctx.rsp = next_fp
    MOVD    R8, R0
    ADD     $8, R0
    MOVD    R0, (mcontext64_ss + regs64_sp)(R4)
    MOVD    R8, (mcontext64_ss + regs64_fp)(R4) // gr[REG_RSP] = fp + 16
    // sigctx.rax = 0 // mark failure
    MOVD   $0, R0
    MOVD   R0, (mcontext64_ss + regs64_x)(R4)

    RESTORE_R19_TO_R28(8*4)
	RESTORE_F8_TO_F15(8*14)
    RET

loop_continue:
    // fp, pc = next_fp, next_pc
    MOVD    R8, R6 // pc = next_pc
    MOVD    R7, R5 // fp = next_fp

    // i += 1
    ADD     $1, R9 
    // if i < FRAMES_TO_CHECK {
    //     goto loop_start
    // }
    MOVD   $FRAMES_TO_CHECK, R0
    CMP    R9, R0
    BL     loop_start

passthrough:
    // exec(func() { state.prevAction.sa_handler(sig, info, ctx) })
    MOVD    (8*1)(RSP), R0 // sig
    MOVD    (8*2)(RSP), R1 // info
    MOVD    (8*3)(RSP), R2   // ctx
    RESTORE_R19_TO_R28(8*4)
	RESTORE_F8_TO_F15(8*14)
    MOVD    ·state+signalState_prevAction+usigactiont___sigaction_u(SB), R7
    B       (R7)


