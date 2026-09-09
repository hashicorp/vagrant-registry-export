# vagrant-registry-export

A CLI for exporting and replicating boxes from an HCP Vagrant Box Registry.

## Commands

### `vagrant-registry-export registry manifest <organization>`

Fetches box, version, provider, and architecture metadata from the HCP Vagrant Box Registry API and prints or writes a JSON manifest.

```sh
vagrant-registry-export registry manifest hashicorp -o manifest.json
vagrant-registry-export registry manifest hashicorp --box bionic64 --box precise64
```

### `vagrant-registry-export registry export [organization]`

Downloads the box files described by a manifest and writes them to a local directory:

```
<dest>/<org>/<box>/versions/<version>/providers/<provider>/<arch>/vagrant.box
```

```sh
vagrant-registry-export registry export --manifest manifest.json --dest ./out
# Fetch the manifest live and export in one step:
vagrant-registry-export registry export hashicorp --dest ./out
```

## Authentication

Private boxes require HCP client credentials:

```sh
vagrant-registry-export --hcp-client-id "$HCP_CLIENT_ID" \
  --hcp-client-secret "$HCP_CLIENT_SECRET" \
  registry export my-org --dest ./out
```

Credentials are resolved in this order:

1. `--hcp-client-id` and `--hcp-client-secret` flags
2. `HCP_CLIENT_ID` and `HCP_CLIENT_SECRET` environment variables
3. `client_id` and `client_secret` keys in the config file (`--config path/to/file.yaml`, default `~/.vagrant-registry-export.yaml`)
