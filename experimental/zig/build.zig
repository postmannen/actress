const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const zbor = b.dependency("zbor", .{
        .target = target,
        .optimize = optimize,
    });

    // The actress library module. Mirrors the Go package
    // github.com/postmannen/actress.
    const actress_mod = b.addModule("actress", .{
        .root_source_file = b.path("src/actress.zig"),
        .target = target,
        .optimize = optimize,
    });
    actress_mod.addImport("zbor", zbor.module("zbor"));

    // The 2actresses example. Mirrors examples/2actresses/main.go.
    const exe = b.addExecutable(.{
        .name = "2actresses",
        .root_module = b.createModule(.{
            .root_source_file = b.path("examples/2actresses/main.zig"),
            .target = target,
            .optimize = optimize,
            .imports = &.{
                .{ .name = "actress", .module = actress_mod },
            },
        }),
    });
    b.installArtifact(exe);

    // Smoke test exercising the routers, cbor and factory process
    // functions that the 2actresses example does not touch.
    const smoke = b.addExecutable(.{
        .name = "smoketest",
        .root_module = b.createModule(.{
            .root_source_file = b.path("examples/smoketest/main.zig"),
            .target = target,
            .optimize = optimize,
            .imports = &.{
                .{ .name = "actress", .module = actress_mod },
                .{ .name = "zbor", .module = zbor.module("zbor") },
            },
        }),
    });
    b.installArtifact(smoke);

    const run_cmd = b.addRunArtifact(exe);
    run_cmd.step.dependOn(b.getInstallStep());
    if (b.args) |args| run_cmd.addArgs(args);
    const run_step = b.step("run", "Run the 2actresses example");
    run_step.dependOn(&run_cmd.step);

    // Library unit tests. The optional test-filter option makes it
    // possible to run a single test: zig build test -Dtest-filter=TestName
    const test_filter = b.option([]const u8, "test-filter", "only run tests matching the filter");
    const lib_tests = b.addTest(.{
        .root_module = actress_mod,
        .filters = if (test_filter) |f| (b.allocator.dupe([]const u8, &.{f}) catch @panic("OOM")) else &.{},
    });
    const run_lib_tests = b.addRunArtifact(lib_tests);
    const test_step = b.step("test", "Run library tests");
    test_step.dependOn(&run_lib_tests.step);
}
