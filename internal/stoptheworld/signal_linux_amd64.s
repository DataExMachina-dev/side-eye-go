// Copyright 2009 The Go Authors. All rights reserved.
// Loosely derived from work in https://github.com/golang/go/blob/9a9dd72d/src/runtime/sys_linux_arm64.s

#include "go_asm.h"
#include "textflag.h"

#define SYS_rt_sigaction	13
#define SYS_gettid		186
#define SYS_rt_sigreturn	15

#define SIGSEGV 11

// For cgo unwinding to work, this function must look precisely like
// the one in glibc. The glibc source code is:
// https://sourceware.org/git/?p=glibc.git;a=blob;f=sysdeps/unix/sysv/linux/x86_64/libc_sigaction.c;h=afdce87381228f0cf32fa9fa6c8c4efa5179065c#l80
// The code that cares about the precise instructions used is:
// https://gcc.gnu.org/git/?p=gcc.git;a=blob;f=libgcc/config/i386/linux-unwind.h;h=5486223d60272c73d5103b29ae592d2ee998e1cf#l49
//
// For gdb unwinding to work, this function must look precisely like the one in
// glibc and must be named "__restore_rt" or contain the string "sigaction" in
// the name. The gdb source code is:
// https://sourceware.org/git/?p=binutils-gdb.git;a=blob;f=gdb/amd64-linux-tdep.c;h=cbbac1a0c64e1deb8181b9d0ff6404e328e2979d#l178
//
// See https://github.com/golang/go/blob/f27a40ce/src/runtime/sys_linux_amd64.s#L460C1-L473C23
TEXT ·sigreturn__sigaction(SB),NOSPLIT|NOFRAME,$0
    MOVQ	$SYS_rt_sigreturn, AX
    SYSCALL
    INT $3	// not reached

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
    // state.snapshotTid = gettid()
    MOVL	$SYS_gettid, AX
    SYSCALL
    MOVL    AX, ·state+signalState_snapshotTid(SB)

    // var sa sigactiont
    ADJSP   $sigactiont__size
    XORQ    AX, AX                        // Zero out AX
    MOVQ    $sigactiont__size, CX         // Load the size of sigactiont
    LEAQ    0(SP), DI                     // DI points to the start of sigactiont

clear_loop:
    TESTQ   CX, CX                        // Check if CX is zero
    JZ      initialize_sa                 // If zero, exit loop
    MOVQ    AX, 0(DI)                     // Store zero at DI
    ADDQ    $8, DI                        // Move to the next 8 bytes
    SUBQ    $8, CX                        // Decrease CX by 8
    JMP     clear_loop                    // Repeat loop

initialize_sa:
    // sa.sa_flags = saFlags
    MOVQ    $const_saFlags, AX
    MOVQ    AX, sigactiont_sa_flags(SP)
    // sa.handler = sigsegvHandler
    LEAQ    ·sigsegvHandler(SB), AX      // Load address of sigsegv_handler
    MOVQ    AX, sigactiont_sa_handler(SP)
    // sa.sa_restorer = runtime·sigreturn__sigaction
    LEAQ    ·sigreturn__sigaction(SB), AX
    MOVQ    AX, sigactiont_sa_restorer(SP)
    // sa.sa_mask = ^uint64(0)
    MOVQ    $0, AX
    NOTQ    AX
    MOVQ    AX, sigactiont_sa_mask(SP)

    // return rt_sigaction(SIGSEGV, &sa, &state.prevAction, unsafe.Sizeof(sa.sa_mask))
    MOVQ    $SIGSEGV, DI
    LEAQ    0(SP), SI
    LEAQ    ·state+signalState_prevAction(SB), DX
    MOVQ    $8, R10
    MOVQ    $SYS_rt_sigaction, AX
    SYSCALL
    ADJSP   $-sigactiont__size
    RET

// resetHandler resets the handler to prevHandler.
//
// func resetHandler() {
//     state.snapshotTid = 0
//     return rt_sigaction(SIGSEGV, &state.prevAction, nil, unsafe.Sizeof(sa.sa_mask))
// }
//
TEXT ·resetHandler(SB), NOSPLIT, $0
    // snapshotTid = 0
    MOVQ    $0, ·state+signalState_snapshotTid(SB)

    // return rt_sigaction(SIGSEGV, &state.prevAction, nil, unsafe.Sizeof(sa.sa_mask))
    MOVQ    $SIGSEGV, DI
    LEAQ    ·state+signalState_prevAction(SB), SI
    MOVQ    $0, DX
    MOVQ    $8, R10
    MOVQ    $SYS_rt_sigaction, AX
    SYSCALL 
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
TEXT ·sigsegvHandler(SB),NOSPLIT|TOPFRAME|NOFRAME,$0
    // func sigsegvHandler(sig uint64, info *siginfo, ctx *ucontext)
    NOP	    SP		// disable vet stack checking
    ADJSP   $24
    MOVQ    DI, -16(SP) // sig
    MOVQ    SI, -8(SP)  // info
    MOVQ    DX, 0(SP)   // ctx

    // tid := gettid()
    MOVL	$SYS_gettid, AX
    SYSCALL

    // if tid != state.snapshotTid {
    //     goto passthrough
    // }
    CMPL    AX, ·state+signalState_snapshotTid(SB)
    JNZ     passthrough

    // sigctx := ctx.uc_mcontext        // BX
    MOVQ    0(SP), BX // BX = ctx
    LEAQ    ucontext_uc_mcontext(BX), BX

    // pc := sigctx.rip                 // CX
    // fp := sigctx.rbp                 // R8
    MOVQ    sigcontext_rip(BX), CX
    MOVQ    sigcontext_rbp(BX), R8

    // i := 0                           // SI
    XORQ    SI, SI

loop_start:
    // if fp == 0 {
    //     goto passthrough
    // }
    TESTQ   R8, R8
    JZ      passthrough

    // next_fp = *(uintptr_t *)(fp)     // R9
    MOVQ    (R8), R9
    // next_pc = *(uintptr_t *)(fp + 8) // R10
    MOVQ    8(R8), R10

    // if pc < state.dereferenceStart {
    //     goto loop_continue
    // }
    MOVQ    ·state+signalState_dereferenceStart(SB), AX
    CMPQ    CX, AX
    JL      loop_continue
    // if pc >= state.dereferenceEnd {
    //     goto loop_continue
    // }
    MOVQ    ·state+signalState_dereferenceEnd(SB), AX
    CMPQ    CX, AX
    JGE     loop_continue

    // sigctx.rbp = next_fp
    MOVQ    R9, sigcontext_rbp(BX)
    // sigctx.rip = next_pc
    MOVQ    R10, sigcontext_rip(BX)
    // sigctx.rsp = fp + 16
    LEAQ    16(R8), AX
    MOVQ    AX, sigcontext_rsp(BX) // gr[REG_RSP] = fp + 16
    // sigctx.rax = 0 // mark failure
    XORQ    AX, AX
    MOVQ    AX, sigcontext_rax(BX)

    MOVQ    -16(SP), DI // sig
    MOVQ    -8(SP), SI  // info
    MOVQ    0(SP), DX   // ctx
    ADJSP	$-24
    RET

loop_continue:
    // fp, pc = next_fp, next_pc
    MOVQ    R9, R8 // fp = next_fp
    MOVQ    R10, CX // pc = next_pc

    // i += 1
    INCQ    SI
    // if i < FRAMES_TO_CHECK {
    //     goto loop_start
    // }
    CMPQ    SI, $FRAMES_TO_CHECK
    JL      loop_start

passthrough:
    // exec(func() { state.prevAction.sa_handler(sig, info, ctx) })
    MOVQ    -16(SP), DI // sig
    MOVQ    -8(SP), SI  // info
    MOVQ    0(SP), DX   // ctx
    ADJSP	$-24
    MOVQ    ·state+signalState_prevAction+sigactiont_sa_handler(SB), AX
    JMP     AX
