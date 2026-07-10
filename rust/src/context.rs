// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

// Context is a small reimplementation of the part of Go's context.Context that
// the actress code relies on: a cancellation tree where cancelling a parent
// cancels all of its children. The process tree mirrors this context tree, so
// stopping a process (or its parent) tears down the whole subtree.
//
// Cancellation is signalled by disconnecting a crossbeam channel. A context
// holds the receiving end (done_rx), and the matching sender is dropped on
// cancel. A disconnected receiver is reported as "ready" by the select! macro,
// which is exactly the behaviour we want from Go's <-ctx.Done().

use crossbeam_channel::{bounded, Receiver, Sender, TryRecvError};
use std::sync::{Arc, Mutex};

// The function returned alongside a cancellable context. Calling it cancels the
// context and, recursively, all of its child contexts.
pub type CancelFn = Arc<dyn Fn() + Send + Sync>;

struct CtxInner {
    // Some until cancelled. Setting it to None drops the sender, which
    // disconnects done_rx and is how we signal "done".
    done_tx: Mutex<Option<Sender<()>>>,
    done_rx: Receiver<()>,
    // Child contexts, cancelled when this context is cancelled.
    children: Mutex<Vec<Arc<CtxInner>>>,
}

#[derive(Clone)]
pub struct Context {
    inner: Arc<CtxInner>,
}

impl Context {
    // Returns a root context that is never cancelled on its own. The equivalent
    // of context.Background().
    pub fn background() -> Context {
        let (tx, rx) = bounded::<()>(0);
        Context {
            inner: Arc::new(CtxInner {
                done_tx: Mutex::new(Some(tx)),
                done_rx: rx,
                children: Mutex::new(Vec::new()),
            }),
        }
    }

    // Derives a new child context from parent and returns it together with a
    // cancel function. The equivalent of context.WithCancel(parent). Cancelling
    // the parent cancels this child as well.
    pub fn with_cancel(parent: &Context) -> (Context, CancelFn) {
        let (tx, rx) = bounded::<()>(0);
        let inner = Arc::new(CtxInner {
            done_tx: Mutex::new(Some(tx)),
            done_rx: rx,
            children: Mutex::new(Vec::new()),
        });

        parent.inner.children.lock().unwrap().push(inner.clone());

        let ctx = Context {
            inner: inner.clone(),
        };
        let cancel: CancelFn = Arc::new(move || cancel_inner(&inner));

        (ctx, cancel)
    }

    // The receiver used with select! to detect cancellation, the equivalent of
    // ctx.Done(). It becomes ready (returns an error) once the context is
    // cancelled.
    pub fn done(&self) -> &Receiver<()> {
        &self.inner.done_rx
    }

    // Reports whether the context has been cancelled.
    pub fn is_done(&self) -> bool {
        matches!(
            self.inner.done_rx.try_recv(),
            Err(TryRecvError::Disconnected)
        )
    }
}

// Cancel a context and then recursively cancel its children. Dropping the
// sender disconnects the matching receiver, which is observed by select!.
fn cancel_inner(inner: &Arc<CtxInner>) {
    {
        let mut tx = inner.done_tx.lock().unwrap();
        *tx = None;
    }

    let children = inner.children.lock().unwrap();
    for child in children.iter() {
        cancel_inner(child);
    }
}
