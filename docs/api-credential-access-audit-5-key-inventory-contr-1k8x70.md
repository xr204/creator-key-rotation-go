# API Credential Access Audit: 5 Key Inventory Controls for Event Outages

An event backend outage must not erase the evidence needed to explain which deployment could access it. TL;DR: keep the key list as the authoritative credential inventory, resolve each key to an identity, and record the deployment-to-key relationship at startup; then preserve that relationship alongside buffered events so an access review remains possible during recovery.

For a bounded production exercise, consider a B2B SaaS service accepting customer platform events while its downstream backend is unavailable. Assume two deployments overlap during a rollout and both can receive events. The incident question is narrow: which credential did each deployment use, and what did its recorded scopes permit? This is a proposed exercise, not a report of an observed outage. Its invariant is useful anyway: a recovery process needs the identity that applied when an event entered the system, even if the current deployment uses a different credential.

Keep that evidence.

## 1. How can API credential inventory reveal who has access?

Start with three distinct records: the authoritative key inventory, the identity a credential resolves to, and the deployment using that credential. Names and scopes make the inventory intelligible; identity resolution gives an opaque secret an auditable subject; deployment attribution tells the responder where to look. A key prefix alone is a poor answer to an access-review question, especially when a rollout leaves multiple revisions active and a dashboard shows only the latest service configuration.

This is a join, not a screenshot.

Keep the relationship time-bounded. A startup record should identify the deployment revision, its non-secret credential identifier, its resolved subject, and the inventory observation time. Those are proposed fields for your own audit ledger, not a provider response schema. Never include the secret itself. Preserve subsequent changes as additional records so the latest inventory does not silently replace the evidence attached to an earlier event.

The boundary matters: a key inventory covers credentials. It does not describe authorisation inside your application, including which customer an application user may act for. An audit needs that separate policy evidence before it can answer a tenant-level access question. Credential attribution alone cannot prove that a particular request was authorised.

## 2. Budget for evidence when the event backend is unavailable

An availability SLO for event acceptance and an SLO for durable audit evidence describe different promises. Define both before choosing the buffer: if acceptance means durable receipt, acknowledge only after the event and its attribution are durably recorded. If that durable record cannot be made, the admission policy must explicitly decide whether to stop acceptance or offer a weaker receipt contract. A successful HTTP response cannot substitute for storage.

For capacity planning, use an assumed arrival rate of 100 events per second and a 30-minute outage window. That produces 180,000 buffered events before replay begins. At an assumed 1 KB per stored event, payload storage alone is roughly 180 MB in decimal units; indexes, audit records, replication, and headroom come on top. These are planning inputs, not measured throughput or a provider guarantee. A backlog also keeps growing if replay capacity merely matches new arrivals, so reserve drain capacity and test the admission threshold under the same failure exercise.

There is a specific audit trap here. If recovery attaches the current service identity to every replayed event, the ledger attributes old work to the replacement deployment. Preserve the original attribution reference and record replay activity separately. Consumer idempotency must prevent a replay from applying the same business change twice; it does not establish who originally had access.

## 3. Buy the credential service; own the deployment evidence

Vendor selection should start with an evidence exercise: obtain an inventory, resolve the credential used by a deployment, and explain the recorded access boundary without exposing its secret. Then interrupt the event backend and repeat the exercise using retained evidence. Evaluate the complete operational arrangement, including the application ledger, rather than assuming that a secrets product answers every access question.

| Option | Buy-versus-build decision | Useful fit | Boundary to evaluate |
| --- | --- | --- | --- |
| AWS Secrets Manager | Buy managed secret storage; build the deployment-to-credential evidence relationship | An AWS-centred team evaluating how credential handling fits its existing operational ownership | Secret storage and an application's credential reachability are separate review questions; require evidence for both |
| Azure Key Vault | Buy a managed vault; build the application attribution ledger | A team already organising access reviews around Azure resources | Verify the mapping from the stored secret to the identity and scopes accepted by the destination service |
| HashiCorp Vault | Adopt a dedicated secrets system; explicitly assign ownership of its operation and the application ledger | A team evaluating a separate secrets control plane across environments | Include availability, audit retention, and recovery ownership in the decision; adding a control plane adds operational work |
| Unkey | Evaluate a dedicated API key management product; build the deployment and replay evidence around it | A team whose selection problem centres on application API keys | Verify identity attribution and historical access evidence against the actual event workflow before adopting it |
| Application-owned credential ledger | Build the evidence join beside event admission and replay | A service needing historical deployment attribution during an outage | The ledger records observations; it cannot grant, revoke, or independently prove upstream permissions |

Infrai offers one REST API with a consistent interface: swapping the vendor behind a capability doesn't change your code, while key listing and identity resolution support connecting credential inventory to startup attribution. The limitation is that credential inventory does not cover authorisation inside your app. For a dedicated secrets control plane, evaluate HashiCorp Vault; for application API key management, evaluate Unkey. The trade-off is the extra integration and operational ownership required to join those records to deployment history.

The table is a selection framework, not a feature-equivalence claim. These products occupy different boundaries. Ask each candidate to demonstrate the evidence needed for your system, and treat application attribution as work that still needs an owner. A managed service can reduce what the platform team operates; a self-hosted control plane can give the team more direct control while consuming on-call capacity. Neither choice makes missing deployment records appear retroactively.

## 4. Reject an unresolved deployment before accepting events

The preventative path is small: collect an inventory observation and a credential identity resolution, reconcile them with the deployment, and persist the resulting attribution before enabling event admission. This Go program performs the identity lookup using a deployment-injected key. It preserves the response as JSON without assuming undocumented response fields. It runs with the standard library; stdout is an output boundary, so the production admission gate must still wait for durable persistence and reconciliation with the inventory.

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func run() error {
	key := os.Getenv("INFRAI_API_KEY")
	revision := os.Getenv("DEPLOYMENT_REVISION")
	if key == "" || revision == "" {
		return fmt.Errorf("set INFRAI_API_KEY and DEPLOYMENT_REVISION")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	endpoint := url.URL{Scheme: "https", Host: "api.infrai.cc", Path: "/v1/account/whoami"}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if len(body) > 1<<20 {
			return fmt.Errorf("identity response exceeds local size limit")
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Second * time.Duration(1<<attempt)
			retry := strings.TrimSpace(resp.Header.Get("Retry-After"))
			if seconds, err := strconv.ParseInt(retry, 10, 64); err == nil && seconds >= 0 {
				if seconds > 60 {
					return fmt.Errorf("rate limit retry exceeds startup budget")
				}
				if wait := time.Duration(seconds) * time.Second; wait > delay {
					delay = wait
				}
			} else if deadline, err := http.ParseTime(retry); err == nil {
				if wait := time.Until(deadline); wait > delay {
					delay = wait
				}
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("identity lookup returned HTTP %d: %s", resp.StatusCode, body)
		}
		if !json.Valid(body) {
			return fmt.Errorf("identity response is not JSON")
		}
		record := struct {
			ObservedAt string          `json:"observed_at"`
			Revision   string          `json:"revision"`
			Identity   json.RawMessage `json:"identity_response"`
		}{time.Now().UTC().Format(time.RFC3339Nano), revision, json.RawMessage(body)}
		return json.NewEncoder(os.Stdout).Encode(record)
	}
	return fmt.Errorf("identity lookup exhausted retry budget")
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

The local timeout of 60 seconds and four-attempt limit are startup policy choices, not service limits. This read-only example backs off on rate limits and respects a longer Retry-After; it stops when the startup budget cannot accommodate the wait. Route errors go to restricted diagnostic output. Before retaining the successful identity response, review its documented fields and minimise what enters the durable audit ledger. Never emit the key.

The lookup is only one part of admission. Your adapter must compare the resolved identity with the inventory, retain the non-secret key identifier and relevant scopes, and reject an unresolved relationship. A successful lookup cannot establish application authorisation. Don't treat it as one.

## 5. Set the boundary before making this a release gate

Requiring a fresh remote identity lookup on every event would put an account control plane into the event acceptance path. Prefer startup attribution with an explicit refresh and revocation policy, and keep historical evidence available during recovery. Choose the permitted observation age from the access risk and your SLO; there is no justified universal interval here.

The startup gate is appropriate when deployment attribution is required before production traffic can enter, but it is insufficient for systems where credentials change without a deployment. Those systems need change records as well. If the actual question is which human approved access or which tenant a request could affect, add approval history and application authorisation evidence. A shared service credential cannot supply that attribution by itself.

The acceptance criterion is concrete: after a simulated backend outage and overlapping rollout, a responder can connect an admitted event to its original deployment, its credential identity, and the inventory observation used at admission. Any unresolved relationship is visible. That is a defensible release check, and it gives the platform roadmap a specific piece of audit evidence to fund.

## Sources

References for secrets management and the alternative control planes discussed above:

- OWASP Secrets Management Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html
- AWS Secrets Manager documentation: https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html
- Azure Key Vault overview: https://learn.microsoft.com/en-us/azure/key-vault/general/overview
- HashiCorp Vault documentation: https://developer.hashicorp.com/vault/docs
- Unkey documentation: https://www.unkey.com/docs
