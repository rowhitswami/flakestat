// Downloads the flakestat binary matching this platform from GitHub Releases.
//
// Runs as a postinstall hook, but every entry point calls ensureBinary() as
// well: installs with --ignore-scripts are common in CI and locked-down
// environments, and the tool should still work there rather than failing with
// a confusing "not found".

"use strict";

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { createHash } = require("node:crypto");
const { execFileSync } = require("node:child_process");

const OWNER = "rowhitswami";
const REPO = "flakestat";

const BIN_DIR = path.join(__dirname, "bin");

/** Maps Node's platform/arch onto the release archive naming in .goreleaser.yaml. */
function target() {
  const platforms = { darwin: "darwin", linux: "linux", win32: "windows" };
  const arches = { x64: "amd64", arm64: "arm64" };

  const goos = platforms[process.platform];
  const goarch = arches[process.arch];

  if (!goos || !goarch) {
    throw new Error(
      `flakestat: unsupported platform ${process.platform}/${process.arch}.\n` +
        `Prebuilt binaries cover macOS, Linux and Windows on x64 and arm64.\n` +
        `Build from source instead: go install github.com/${OWNER}/${REPO}/cmd/${REPO}@latest`
    );
  }

  return {
    goos,
    goarch,
    ext: goos === "windows" ? "zip" : "tar.gz",
    binName: goos === "windows" ? "flakestat.exe" : "flakestat",
  };
}

function binPath() {
  return path.join(BIN_DIR, target().binName);
}

function version() {
  return require("./package.json").version;
}

async function fetchBuffer(url) {
  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok) {
    throw new Error(`GET ${url} -> ${res.status} ${res.statusText}`);
  }
  return Buffer.from(await res.arrayBuffer());
}

// Fails closed. Warning and installing anyway means anyone who can drop the
// checksums request has silently downgraded this to no verification.
// FLAKESTAT_SKIP_CHECKSUM=1 opts out deliberately; it is never the default.
async function verifyChecksum(archiveName, buf, baseUrl) {
  if (process.env.FLAKESTAT_SKIP_CHECKSUM === "1") {
    console.warn("flakestat: FLAKESTAT_SKIP_CHECKSUM=1, installing without verification");
    return;
  }

  let sums;
  try {
    sums = (await fetchBuffer(`${baseUrl}/checksums.txt`)).toString("utf8");
  } catch (err) {
    throw new Error(
      `flakestat: could not fetch checksums.txt from ${baseUrl} (${err.message})\n` +
        "  Refusing to install an unverified binary. Retry, or set\n" +
        "  FLAKESTAT_SKIP_CHECKSUM=1 if you accept the risk."
    );
  }

  const line = sums.split("\n").find((l) => l.trim().endsWith(archiveName));
  if (!line) {
    throw new Error(
      `flakestat: no checksum listed for ${archiveName} in checksums.txt\n` +
        "  Refusing to install an unverified binary."
    );
  }

  const expected = line.trim().split(/\s+/)[0];
  const actual = createHash("sha256").update(buf).digest("hex");

  if (expected !== actual) {
    throw new Error(
      `flakestat: checksum mismatch for ${archiveName}\n  expected: ${expected}\n  actual:   ${actual}`
    );
  }
}

async function download() {
  const { goos, goarch, ext, binName } = target();
  const v = version();
  const archive = `${REPO}_${v}_${goos}_${goarch}.${ext}`;
  const baseUrl = `https://github.com/${OWNER}/${REPO}/releases/download/v${v}`;

  console.error(`flakestat: downloading ${REPO} v${v} (${goos}/${goarch})`);

  const buf = await fetchBuffer(`${baseUrl}/${archive}`);
  await verifyChecksum(archive, buf, baseUrl);

  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "flakestat-"));
  const archivePath = path.join(tmp, archive);

  try {
    fs.writeFileSync(archivePath, buf);

    // bsdtar ships with Windows 10+ and handles zip as well as tar.gz, so one
    // command covers every platform without pulling in an extraction library.
    execFileSync("tar", ["-xf", archivePath, "-C", tmp], { stdio: "ignore" });

    fs.mkdirSync(BIN_DIR, { recursive: true });
    fs.copyFileSync(path.join(tmp, binName), path.join(BIN_DIR, binName));
    fs.chmodSync(path.join(BIN_DIR, binName), 0o755);
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }

  console.error(`flakestat: installed to ${path.join(BIN_DIR, binName)}`);
}

/** Downloads the binary if it is not already present. */
async function ensureBinary() {
  const override = process.env.FLAKESTAT_BINARY;
  if (override) {
    if (!fs.existsSync(override)) {
      throw new Error(`flakestat: FLAKESTAT_BINARY=${override} does not exist`);
    }
    return override;
  }

  if (fs.existsSync(binPath())) return binPath();

  if (process.env.FLAKESTAT_SKIP_DOWNLOAD) {
    throw new Error(
      "flakestat: binary missing and FLAKESTAT_SKIP_DOWNLOAD is set"
    );
  }

  await download();
  return binPath();
}

module.exports = { ensureBinary, binPath, target, download };

if (require.main === module) {
  // A failed postinstall must not break the consumer's whole install: the bin
  // shim retries the download on first use.
  ensureBinary().catch((err) => {
    console.warn(`flakestat: ${err.message}`);
    console.warn("flakestat: will retry on first use");
    process.exit(0);
  });
}
