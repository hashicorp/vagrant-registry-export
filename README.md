# vagrant-registry-export

A CLI for exporting and replicating boxes from an
[HCP Vagrant Box Registry](https://developer.hashicorp.com/hcp/docs/vagrant).

It can generate a JSON manifest of a registry's boxes and download the box
files to a local directory that mirrors the registry layout. Each exported box
also gets a Vagrant-compatible `metadata.json`, so the result can be consumed
directly with `vagrant box add`.

> Throughout this document, _registry_ refers to an HCP Vagrant Box Registry,
> which is identified by its organization (or username).

## Installation

### Install a prebuilt release

1. Open the [latest GitHub release](https://github.com/hashicorp/vagrant-registry-export/releases/latest).
2. Download the asset matching your operating system and processor architecture:

| Operating system | amd64 (x86-64)                          | arm64 (64-bit ARM)                      |
| ---------------- | --------------------------------------- | --------------------------------------- |
| macOS            | `vagrant-registry-export_darwin_amd64`  | `vagrant-registry-export_darwin_arm64`  |
| Linux            | `vagrant-registry-export_linux_amd64`   | `vagrant-registry-export_linux_arm64`   |
| Windows          | `vagrant-registry-export_windows_amd64` | `vagrant-registry-export_windows_arm64` |

The release also includes a `SHA256SUMS` file with checksums for the binaries. 3. On macOS or Linux, open a terminal in the download directory and grant the binary
permission to run:

```sh
chmod +x ./vagrant-registry-export_<os>_<arch>
```

Replace `<os>` and `<arch>` with the values from the asset name you downloaded.
You can then run it from that directory, for example:

```sh
./vagrant-registry-export_darwin_arm64 version
```

On Windows, rename the downloaded asset to add the `.exe` extension, then run it
from PowerShell or Command Prompt.

On macOS, Gatekeeper may quarantine a downloaded binary. If macOS blocks it, remove
the quarantine attribute from the binary and try again:

```sh
xattr -d com.apple.quarantine ./vagrant-registry-export_<os>_<arch>
```

### Build from source

Build the binary with the provided Makefile:

```sh
make build          # builds bin/vagrant-registry-export for the host platform
make build-all      # builds darwin/linux/windows for amd64 and arm64
make build VERSION=v1.2.3  # stamp a release version into the binary
make build VERSION=v1.2.3 VERSION_PRERELEASE=rc.1 VERSION_METADATA=build.42
```

Or build directly with Go:

```sh
go build -o vagrant-registry-export .
```

## Releases

Run the `Release` workflow from GitHub Actions with a version such as `v1.2.3`.
Optional prerelease and metadata inputs are appended to the version; for example,
`v1.2.3`, `rc.1`, and `build.42` produce the release tag
`v1.2.3-rc.1+build.42`. The workflow publishes the platform binaries and a
`SHA256SUMS` file. Supplying a prerelease identifier marks the GitHub release as
a prerelease.

## Commands

### `version`

Prints the version of the binary.

```sh
vagrant-registry-export version
```

### `registry manifest <registry>`

Fetches box, version, provider, and architecture metadata from the HCP Vagrant
Box Registry API and prints or writes a JSON manifest. The manifest can later be
passed to `registry export` so exporting does not need to re-query the API.

```sh
# Print the manifest to stdout
vagrant-registry-export registry manifest hashicorp

# Write the manifest to a file
vagrant-registry-export registry manifest hashicorp -o manifest.json

# Limit the manifest to specific boxes
vagrant-registry-export registry manifest hashicorp --box bionic64 --box precise64
```

Flags:

| Flag             | Default      | Description                                           |
| ---------------- | ------------ | ----------------------------------------------------- |
| `-o`, `--output` | `-` (stdout) | File to write the manifest to (`-` for stdout).       |
| `--box`          | (all)        | Limit the manifest to specific box names. Repeatable. |

### `registry export [registry]`

Downloads the box files described by a manifest and replicates them in a local
directory tree that mirrors the registry layout:

```
<dest>/<registry>/<box>/versions/<version>/providers/<provider>/<arch>/vagrant.box
```

Provide either a registry name (to fetch the manifest live from HCP) or
`--manifest` pointing to a previously generated manifest JSON file.

```sh
# Fetch the manifest live and export in one step
vagrant-registry-export registry export hashicorp --dest ./out

# Export from a previously generated manifest
vagrant-registry-export registry export --manifest manifest.json --dest ./out

# Export only selected providers/architectures, four downloads at a time
vagrant-registry-export registry export hashicorp \
  --provider virtualbox --arch amd64 --concurrency 4
```

Flags:

| Flag                        | Default    | Description                                                        |
| --------------------------- | ---------- | ------------------------------------------------------------------ |
| `--manifest`                |            | Path to an existing manifest JSON file (instead of fetching live). |
| `-d`, `--dest`              | `./export` | Destination directory to replicate box files into.                 |
| `--force`                   | `false`    | Re-download files even if they already exist on disk.              |
| `-c`, `--concurrency`       | `1`        | Number of box files to download in parallel.                       |
| `--skip-vagrant-metadata`   | `false`    | Skip writing a per-box `metadata.json`.                            |
| `--box`, `--boxes`          | (all)      | Limit export to specific box name(s).                              |
| `--version`, `--versions`   | (all)      | Limit export to specific version(s).                               |
| `--provider`, `--providers` | (all)      | Limit export to specific provider(s), e.g. `virtualbox`.           |
| `--arch`, `--archs`         | (all)      | Limit export to specific architecture(s), e.g. `amd64`.            |

The `--box`/`--boxes` (and the other filter pairs) are aliases for one another;
the singular form is repeatable (`--box a --box b`) while the plural form also
accepts a comma-separated list (`--boxes a,b`). Filters are case-insensitive.

Files that are already present on disk are skipped (and their checksum verified
when known) unless `--force` is given.

#### Adding exported boxes to Vagrant

Unless `--skip-vagrant-metadata` is set, a `metadata.json` in Vagrant's own
catalog format is written for each box alongside its downloaded files. Add a box
to Vagrant by pointing at that file:

```sh
vagrant box add ./out/hashicorp/bionic64/metadata.json
```

Re-running an export with different filters merges into the existing
`metadata.json` rather than overwriting it, so entries accumulate across runs.

## Authentication

Public registries and boxes can be exported without credentials. Private boxes
require HCP service principal client credentials:

```sh
vagrant-registry-export --hcp-client-id "$HCP_CLIENT_ID" \
  --hcp-client-secret "$HCP_CLIENT_SECRET" \
  registry export my-org --dest ./out
```

Credentials are resolved in this order:

1. `--hcp-client-id` and `--hcp-client-secret` flags
2. `HCP_CLIENT_ID` and `HCP_CLIENT_SECRET` environment variables
3. `client_id` and `client_secret` keys in the config file
   (`--config path/to/file.yaml`, default `$HOME/.vagrant-registry-export.yaml`)

## Development

```sh
make test           # run tests
make test-coverage  # run tests with coverage
make vet            # run go vet
make fmt            # format code
make fmt-check      # verify formatting
```
