// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// This file has no counterpart in the Go version. Go provides the context
// package in it's standard library, and the Go version of actress uses
// context.WithCancel to build a tree of cancelable contexts that follow
// the process tree. Zig got no context package, so a minimal equivalent
// is implemented here on top of std.Io task cancelation.
//
// The mapping from Go to Zig works like this:
//   - Each context holds an Io.Group. The process function belonging to
//     a process, and any extra tasks the process starts (the equivalent
//     of the `go func()` statements in the Go code), are spawned into
//     the group of the process own context.
//   - Cancel() marks the context, and all child contexts, as canceled,
//     and then cancels the groups. Canceling a group makes any blocking
//     std.Io call inside the tasks return error.Canceled.
//   - Where the Go code does `select { case <-ctx.Done(): return }` the
//     Zig code instead does `... catch return` on the blocking queue
//     operation, since the cancelation is delivered through the blocking
//     call itself.
//
// Individual contexts can be freed with deinit while the rest of the tree
// stays alive, which is what happens when a stopped dynamic or custom
// process is freed. The parent and children links of the whole tree are
// protected by a single tree lock shared by all contexts of the tree, so
// detaching one context can not race with a cancel walking the tree. The
// tree lock lives in a small reference counted struct, so it stays alive
// until the last context of the tree is freed, no matter in what order
// the contexts are freed.

const std = @import("std");
const Io = std.Io;

// The state shared by every context in one tree: the lock protecting the
// parent/children links, and a reference count so the struct itself can
// be freed when the last context referencing it is freed.
const contextTree = struct {
    mu: Io.Mutex = .init,
    refs: std.atomic.Value(usize),
};

pub const Context = struct {
    allocator: std.mem.Allocator,
    io: Io,
    // The shared tree state, see contextTree.
    tree: *contextTree,
    // Set to true when cancel have been called. Also used as a guard so
    // the group is only canceled once (Io.Group.cancel is not threadsafe),
    // and so a task can not deadlock by awaiting it's own group when the
    // context already is canceled.
    canceled: std.atomic.Value(bool) = .init(false),
    // The number of cancel calls that have collected this context from the
    // tree and not yet finished canceling it's group. deinit waits for
    // this to reach zero, so a cancel in progress can never touch a freed
    // context.
    cancelBorrows: std.atomic.Value(usize) = .init(0),
    // The group holding all the tasks started within this context.
    group: Io.Group = .init,
    parent: ?*Context = null,
    children: std.ArrayList(*Context) = .empty,

    // Background will prepare and return a root context, like the Go
    // context.Background function. The context must be freed with deinit
    // after it have been canceled.
    pub fn Background(allocator: std.mem.Allocator, io: Io) !*Context {
        const tree = try allocator.create(contextTree);
        errdefer allocator.destroy(tree);
        tree.* = .{ .refs = .init(1) };

        const ctx = try allocator.create(Context);
        ctx.* = .{
            .allocator = allocator,
            .io = io,
            .tree = tree,
        };
        return ctx;
    }

    // WithCancel will prepare and return a child context of the parent
    // given as input, like the Go context.WithCancel function. Canceling
    // the parent will also cancel the child. The cancel function of the
    // Go version is the cancel method on the returned context.
    pub fn WithCancel(parent: *Context) !*Context {
        const ctx = try parent.allocator.create(Context);
        errdefer parent.allocator.destroy(ctx);
        ctx.* = .{
            .allocator = parent.allocator,
            .io = parent.io,
            .tree = parent.tree,
            .parent = parent,
        };

        parent.tree.mu.lockUncancelable(parent.io);
        defer parent.tree.mu.unlock(parent.io);
        try parent.children.append(parent.allocator, ctx);
        _ = parent.tree.refs.fetchAdd(1, .acq_rel);

        return ctx;
    }

    // cancel will mark the context and all it's children as canceled, and
    // cancel all the tasks started within them. Blocking std.Io calls in
    // the tasks will return error.Canceled.
    // Calling cancel a second time is a no-op, and it is safe for a task
    // to call cancel on it's own already canceled context.
    //
    // The subtree is first collected under the tree lock, where each
    // context is marked canceled and borrowed. The groups are then
    // canceled after the lock is released, since canceling a group waits
    // for it's tasks to finish, and a finishing task may itself need the
    // tree lock (for example by creating a new process). A context that
    // already is marked canceled is skipped, the cancel call that marked
    // it owns canceling it's group.
    pub fn cancel(ctx: *Context) void {
        var list: std.ArrayList(*Context) = .empty;
        defer list.deinit(ctx.allocator);

        ctx.tree.mu.lockUncancelable(ctx.io);
        collectSubtree(ctx, &list);
        ctx.tree.mu.unlock(ctx.io);

        for (list.items) |c| {
            c.group.cancel(c.io);
            _ = c.cancelBorrows.fetchSub(1, .release);
        }
    }

    // Mark the context and all it's children as canceled, and collect the
    // ones this cancel call is responsible for into the list. Must be
    // called with the tree lock held.
    fn collectSubtree(ctx: *Context, list: *std.ArrayList(*Context)) void {
        if (!ctx.canceled.swap(true, .acq_rel)) {
            _ = ctx.cancelBorrows.fetchAdd(1, .acq_rel);
            list.append(ctx.allocator, ctx) catch |err| {
                // Without the entry in the list the group of the context
                // would never be canceled and it's tasks never stopped,
                // there is no way to continue from that.
                std.debug.panic("Context.cancel: allocation failed: {s}", .{@errorName(err)});
            };
        }
        for (ctx.children.items) |child| {
            collectSubtree(child, list);
        }
    }

    // isCanceled reports if the context have been canceled.
    pub fn isCanceled(ctx: *Context) bool {
        return ctx.canceled.load(.acquire);
    }

    // spawn will start a new task within the context. This is the
    // equivalent of the `go` statement in the Go version. The function
    // must have a return type that can coerce to Io.Cancelable!void.
    pub fn spawn(ctx: *Context, function: anytype, args: std.meta.ArgsTuple(@TypeOf(function))) !void {
        try ctx.group.concurrent(ctx.io, function, args);
    }

    // deinit will free the memory of the context itself. The context must
    // be canceled before calling deinit, so none of it's tasks are running
    // anymore. The context is detached from it's parent, and any children
    // still alive are orphaned, they are freed by their own owners.
    pub fn deinit(ctx: *Context) void {
        ctx.tree.mu.lockUncancelable(ctx.io);
        if (ctx.parent) |parent| {
            for (parent.children.items, 0..) |c, i| {
                if (c == ctx) {
                    _ = parent.children.swapRemove(i);
                    break;
                }
            }
        }
        for (ctx.children.items) |child| {
            child.parent = null;
        }
        ctx.tree.mu.unlock(ctx.io);

        // A cancel call may have collected this context from the tree
        // right before it was detached above, wait for it to be done with
        // the context before the memory is freed.
        while (ctx.cancelBorrows.load(.acquire) != 0) {
            ctx.io.sleep(.fromMilliseconds(1), .awake) catch {};
        }

        const allocator = ctx.allocator;
        const tree = ctx.tree;
        ctx.children.deinit(allocator);
        allocator.destroy(ctx);
        if (tree.refs.fetchSub(1, .acq_rel) == 1) {
            allocator.destroy(tree);
        }
    }
};
