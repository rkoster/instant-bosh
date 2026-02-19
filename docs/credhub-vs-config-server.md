# CredHub vs Config-Server: API Compatibility Research

## Overview

This document summarizes research into the compatibility between the CredHub CLI and the BOSH config-server, conducted while attempting to enable `credhub` CLI usage with instant-bosh's config-server backend.

## Background

instant-bosh deploys a BOSH director with a **config-server** for credential management. Users wanted to use the `credhub` CLI to interact with stored credentials, but encountered errors.

## Key Finding: They Are Different Systems

**The CredHub CLI and config-server are fundamentally different systems with incompatible APIs.**

The config-server (`cloudfoundry/config-server`) is a minimal credential storage server designed specifically for BOSH director's internal use. CredHub (`cloudfoundry/credhub`) is a full-featured credential management system with a CLI and richer API.

## API Comparison

| Feature | config-server | CredHub |
|---------|---------------|---------|
| API prefix | `/v1/data` | `/api/v1/data` |
| Get by name | `GET /v1/data?name=X&current=true` | `GET /api/v1/data?name=X&current=true` |
| Get by ID | `GET /v1/data/{id}` | `GET /api/v1/data/{uuid}` |
| Find by path | **Not supported** | `GET /api/v1/data?path=X` |
| Find by name-like | **Not supported** | `GET /api/v1/data?name-like=X` |
| `/info` endpoint | **Not supported** | Returns app info and auth-server URL |
| Certificates API | **Not supported** | `/api/v1/certificates/` |
| Bulk regenerate | **Not supported** | `/api/v1/bulk-regenerate` |
| Interpolate | **Not supported** | `/api/v1/interpolate` |
| Response format | `{"id":..., "name":..., "value":...}` | `{"data":[{...}]}` (wrapped in array) |

## How BOSH Director Uses Config-Server

Looking at the BOSH director source code (`bosh/src/bosh-director/lib/bosh/director/config_server/config_server_http_client.rb`):

```ruby
class ConfigServerEnabledHTTPClient
  def get_by_id(id)
    uri = URI.join(@config_server_uri, "v1/data/#{id}")
    @http.get(uri.request_uri)
  end

  def get(name)
    uri = build_base_uri
    uri.query = URI.encode_www_form([["name", name], ["current", "true"]])
    @http.get(uri.request_uri)
  end

  def post(body)
    uri = build_base_uri
    @http.post(uri.path, JSON.dump(body), {'Content-Type' => 'application/json'})
  end

  private

  def build_base_uri
    URI.join(@config_server_uri, 'v1/data')
  end
end
```

**BOSH director uses the simple `/v1/data` API** - it does NOT use the CredHub CLI or the `/api/v1/data` API. This is why config-server works perfectly for BOSH deployments but not with the CredHub CLI.

## Why CredHub Works With Both

CredHub implements BOTH APIs:
- The `/api/v1/data` endpoints (CredHub native API, used by `credhub` CLI)
- The `/v1/data` endpoints (for backwards compatibility with BOSH director)

This allows CredHub to serve as a drop-in replacement for config-server while also providing CLI access.

## What We Attempted

We modified config-server to add CredHub CLI compatibility:

1. **Added `/info` endpoint** - Returns CredHub-compatible JSON with app name/version and auth-server URL
2. **Added `/api/v1/data` routes** - Mapped to existing handlers
3. **Fixed path extraction** - Handle both `/v1/data/ID` and `/api/v1/data/ID` patterns

However, full compatibility would require:
- Implementing find-by-path functionality (new database queries)
- Implementing find-by-name-like functionality
- Changing response format to match CredHub's `{"data":[...]}` wrapper
- Adding certificate management endpoints
- And more...

## Options

### Option 1: Don't Use CredHub CLI with config-server

The simplest approach. Use alternative methods to query credentials:

```bash
# Direct API call using curl
eval "$(ibosh incus print-env)"
curl -sk -H "Authorization: Bearer $TOKEN" \
  "https://$BOSH_ENVIRONMENT:8081/v1/data?name=/my-credential&current=true"
```

Or write a simple wrapper script that speaks the config-server API.

### Option 2: Switch to CredHub

Deploy actual CredHub instead of config-server. CredHub is a superset that supports:
- BOSH director's `/v1/data` API
- CredHub CLI's `/api/v1/data` API
- All advanced features (find, certificates, bulk operations, etc.)

This requires:
- Deploying CredHub release alongside BOSH
- More resources (CredHub is a Java application)
- Additional configuration

### Option 3: Build Full Compatibility Layer

Extend config-server to fully implement the CredHub API. This is significant work:
- Add find-by-path and find-by-name-like queries
- Wrap responses in `{"data":[...]}` format
- Add certificate management endpoints
- Add bulk operations
- Match all response field names exactly

## Recommendation

For instant-bosh's use case (lightweight local BOSH development):

- **Keep config-server** for BOSH director operations (it works perfectly)
- **Don't rely on CredHub CLI** - use direct API calls or a custom script if needed
- **Consider CredHub** only if full CLI support is required and resource constraints allow

## References

- [cloudfoundry/config-server](https://github.com/cloudfoundry/config-server) - Simple config server
- [cloudfoundry/credhub](https://github.com/cloudfoundry/credhub) - Full credential management
- [cloudfoundry/credhub-cli](https://github.com/cloudfoundry/credhub-cli) - CredHub CLI
- [CredHub API Documentation](https://docs.cloudfoundry.org/api/credhub/version/main/)
- BOSH Director config_server client: `bosh/src/bosh-director/lib/bosh/director/config_server/`
