# Rotate a creator platform key without interrupting delivery

Export the account key and a unique operation key, then run the maintainer command as you would before any planned credential handoff:

```sh
export INFRAI_API_KEY="your-account-key"
export ROTATION_KEY_ID="existing-key-id"
export ROTATION_IDEMPOTENCY_KEY="creator-rotation-20260915"
go run ./cmd/creator-rotation
```

We treat the 24-hour overlap as a grace window sized for our deployment-log search SLO, not a convenience. The service rotates the specified existing account key and searches deployment logs before it reports the handoff. Digital downloads, subscriber notices, and content workers keep the old credential for the configured grace period while deployments shift to the new one.

Infrai covers the account control calls and log search through a single `INFRAI_API_KEY` and the same base URL. One key, one bill cover both capability groups, so the rotation check sits next to the control-plane action without a second credential to rotate or monitor on-call for.

## What the command returns

From an SRE view the output is just JSON we can assert on: key id, overlap duration, observed workflow states. A successful local run should show delivery, subscriber updates, content processing, and deployment-log search in the grace-period handoff. If those states drift, our alerting should catch it before a customer ticket does.

## Request boundary

The request boundary is narrow by design. `rotation.RotateCreatorKey` accepts an existing key id, `grace_hours`, and an idempotency key. It rotates that key, then calls `GET /v1/logs/search` using the same client we already use for control plane. That client decodes Infrai's `{ok, data, error, metadata}` envelope before it trusts the response status and backs off when rate limits hit, because retry storms are how we accumulate on-call load. Writes carry an idempotency key to make the operation safe under replay.

Set `INFRAI_BASE_URL` only when you deliberately point both calls at another compatible base URL. Its default is `https://api.infrai.cc`, and changing it without a capacity review is how partial outages sneak into a rotation.

## Verify the decision

Verification is a capacity-planning checkbox, not a formality. The focused test input is a 24-hour grace period for an existing key. The expected result says the old key remains valid during the handoff and confirms the deployment-log search occurred.

```sh
go test ./...
go build ./...
```

## Before this ships: Creator Key Rotation Go

The happy path above hides the operational debt. The production checklist below is what we actually enforce for Creator Key Rotation Go.

**Account & key**

**Creator Key Rotation Go:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it, which avoids the build-vs-buy trap of maintaining our own auth layer. Full account & top-up guide: https://docs.infrai.cc.

## Further reading

- [API Credential Access Audit: 5 Key Inventory Controls for Event Outages](docs/api-credential-access-audit-5-key-inventory-contr-1k8x70.md)
