const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const lib = b.addStaticLibrary(.{
        .name = "actress",
        .root_source_file = b.path("src/actress.zig"),
        .target = target,
        .optimize = optimize,
    });

    b.installArtifact(lib);

    // Module for external use
    _ = b.addModule("actress", .{
        .root_source_file = b.path("src/actress.zig"),
    });

    // Tests
    const lib_unit_tests = b.addTest(.{
        .root_source_file = b.path("src/actress.zig"),
        .target = target,
        .optimize = optimize,
    });

    const run_lib_unit_tests = b.addRunArtifact(lib_unit_tests);

    const test_step = b.step("test", "Run unit tests");
    test_step.dependOn(&run_lib_unit_tests.step);

    // Examples
    const two_actresses_exe = b.addExecutable(.{
        .name = "two_actresses",
        .root_source_file = b.path("examples/two_actresses.zig"),
        .target = target,
        .optimize = optimize,
    });
    two_actresses_exe.root_module.addImport("actress", b.modules.get("actress").?);
    b.installArtifact(two_actresses_exe);

    const httpget_exe = b.addExecutable(.{
        .name = "httpget",
        .root_source_file = b.path("examples/httpget.zig"),
        .target = target,
        .optimize = optimize,
    });
    httpget_exe.root_module.addImport("actress", b.modules.get("actress").?);
    b.installArtifact(httpget_exe);
}
