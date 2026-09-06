#!/usr/bin/env node
// Thin shim: make sure the binary is present, then hand over to it.

"use strict";

const { spawn } = require("node:child_process");
const { ensureBinary } = require("../install.js");

ensureBinary()
  .then((bin) => {
    const child = spawn(bin, process.argv.slice(2), { stdio: "inherit" });

    // Forward signals so Ctrl-C reaches the test command flakestat is running.
    for (const sig of ["SIGINT", "SIGTERM"]) {
      process.on(sig, () => child.kill(sig));
    }

    child.on("error", (err) => {
      console.error(`flakestat: ${err.message}`);
      process.exit(1);
    });

    child.on("close", (code, signal) => {
      // Preserve the exit code: CI gates depend on it.
      if (signal) {
        process.kill(process.pid, signal);
        return;
      }
      process.exit(code ?? 0);
    });
  })
  .catch((err) => {
    console.error(`flakestat: ${err.message}`);
    process.exit(1);
  });
