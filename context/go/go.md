# GO CODE STYLE

**For reviewers / agents:** cite the stable `RULE-*` id when flagging a violation. Fix violations
rather than debating them. Prefer automatic fixes where a tool covers the rule (tagged `[gofmt]`,
`[goimports]`, `[vet]`); everything tagged `[review]` is a manual check.

**Scope:** general Go style. Topic-specific rules live in sibling guides — see **SUB-GUIDES** at the
end (`go-tests.md`, `go-goroutines.md`, `../sql/sql.md`); they are equally mandatory.

---

## BASELINE

All code MUST follow official Go conventions:

* https://go.dev/doc/effective_go
* https://google.github.io/styleguide/go/

Where this document conflicts with Go conventions, Go conventions win — **EXCEPT** acronym /
initialism casing, which follows **RULE-NAME-002** (a deliberate, documented local override of the
Go "initialisms" convention).

---

## NAMING

**RULE-NAME-001** — Case `[review]`

* Exported identifiers MUST use PascalCase (`GameVote`); unexported MUST use camelCase (`gameVote`).
* Underscores are NOT allowed in identifiers. Exceptions (Go / tooling conventions): Go test
  function names (`TestType_Method`, `Test_helper`) and test-case meta fields (e.g. `_id`) — see
  `go-tests.md`.

**RULE-NAME-002** — Acronyms / initialisms `[review]`

* Identifier casing SUPERSEDES the conventional casing of an acronym / initialism.
  Treat every acronym (OTP, ID, TTL, SES, SQS, UUID, URL, URI, API, HTTP, HTML, JSON, DB, …) as an ordinary word:
  * PascalCase symbols (exported types, funcs, methods, fields, consts): capitalise only the first letter → `Otp`, `Id`, `Ttl`, `Uuid`, `Url`, `Json`, `Db`, `SesSender`, `HttpClient`
  * camelCase symbols (unexported vars, params, fields): a leading acronym is fully lower-case; a non-leading acronym takes the PascalCase form → `otpHash`, `userId`, `otpTtl`, `deviceUuid`, `sqsBounceQueueUrl`
  * Holds no matter how many acronyms collide → `LoginIntentOtpTtl` (NOT `LoginIntentOTPTTL`), `SesAccessKeyId` (NOT `SESAccessKeyID`)
* EXACTLY ONE spelling per concept, repo-wide (this REPLACES the old "consistency within a file")
* SOLE EXCEPTION — identifiers we do not own: interface methods, fields, and functions from the stdlib / third-party / generated code MUST match their upstream spelling exactly
  * e.g. `ServeHTTP` (implements `http.Handler`); reading `r.RequestURI` is upstream, not ours
  * struct tags / wire formats are strings, not identifiers → follow the external contract (`json:"otp_hash"`), unaffected
* This is a deliberate LOCAL OVERRIDE of the Go "initialisms" convention (Go Code Review Comments / Google Go Style Guide), chosen because it is mechanical (needs no initialism dictionary)

**RULE-NAME-003** — Verbose names `[review]`

* Names MUST be verbose and descriptive.
* Abbreviations are allowed ONLY for: well-known acronyms (cased per RULE-NAME-002), single-letter
  method receivers, and the error variable `err`.
* Single-letter loop and local variables are violations too (`g` → `hookGroup`,
  `h` → `hook`, `i` → `index`). Exceptions: method receivers, `err`, and the
  idiomatic HTTP handler parameters `w http.ResponseWriter, r *http.Request`.

```go
// GOOD
cooldownErr
retryAfterSeconds
intentId

// BAD
ce
ras
iId
```

**RULE-NAME-004** — No boolean operator in method name `[review]`

* A method name MUST NOT encode an "or" / "and" branching choice (`ResolveXOrY`, `FindOrCreate`,
  `GetXAndY`). This is a code smell: the name hides two distinct behaviors behind one signature,
  forcing the caller to know which branch fires. Split into separate, single-purpose methods
  instead — the caller decides which one to call.

```go
// BAD: name bakes an "or" branch into one method
func resolveSessionOrLatest(s *session.Store, request mcp.CallToolRequest, agent session.Agent) (*session.Session, error) {
    args := request.GetArguments()
    id, _ := args["id"].(string)
    title, _ := args["title"].(string)
    if id != "" || title != "" {
        return resolveSession(s, request)
    }

    sess, ok := s.Last(agent)
    if !ok {
        return nil, errors.New("resolveSessionOrLatest: No sessions found")
    }
    return sess, nil
}

// GOOD: two single-purpose methods, caller picks
func resolveSession(s *session.Store, request mcp.CallToolRequest) (*session.Session, error) {
    ...
}

func resolveSessionLatest(s *session.Store, agent session.Agent) (*session.Session, error) {
    sess, ok := s.Last(agent)
    if !ok {
        return nil, errors.New("resolveSessionLatest: No sessions found")
    }
    return sess, nil
}
```

**RULE-BOOL-001** — Boolean predicates `[review]`

* Booleans MUST read as predicates: `isX`, `hasX`, `shouldX` (e.g. `isActive`, `hasAccess`, `shouldRetry`).

**RULE-BOOL-002** — Boolean config fields `[review]`

* Boolean configuration/feature fields on models are named `Is<X>Active` (e.g. `IsTestModeActive`),
  not bare adjectives (`TestMode`, `Enabled`).

**RULE-RECEIVER-001** — Receiver names `[review]`

* Method receivers MUST be short and consistent within a type: `s` (service), `c` (config),
  `r` (request), etc. The same receiver name MUST be used across all methods of a type.

**RULE-INTERFACE-001** — No `I` prefix `[review]`

```go
type Service interface {}  // correct
type IService interface {} // forbidden
```

**RULE-PKG-001** — Package names `[review]`

* lowercase, no underscores, short and meaningful.

---

## FILE & TYPE STRUCTURE

**RULE-FILE-001** — Element order `[review]`

A file is ordered as a file-level preamble, then one self-contained block per type, then free
functions:

1. Type aliases
2. Constants
3. Variables
4. Type blocks — one block per type (struct, interface, named type); order the blocks alphabetically
   by type name. Each block keeps the type together with everything defined on it, in this order:
   1. the `type` declaration
   2. its constructor(s) / factories — `New<Type>()` directly below the declaration
   3. its non-exported (private) methods, alphabetical
   4. its exported (public) methods, alphabetical
5. Package-level functions not bound to a type — non-exported first, then exported, each group alphabetical.

Do NOT collect all methods of the file into one global group; keep each type's methods in its own
block. Example: `type A`, `NewA`, A's methods, then `type B`, `NewB`, B's methods, then free functions.

**RULE-STRUCT-001** — Field grouping `[review]`

* Unexported (private) fields first, then EXACTLY one blank line, then exported (public) fields.

**RULE-STRUCT-002** — Field sorting `[review]`

* Sort fields alphabetically within each group. Exception: `Id` is always the first public field.

**RULE-STRUCT-003** — No interface-typed struct fields `[review]`

* Struct fields MUST be concrete types. An interface-typed field is a design decision that needs
  explicit approval — stop and ask before introducing one.
* Interfaces belong at function boundaries (parameters), not in storage. Model reader/writer
  capabilities with the stdlib `io.Reader` / `io.Writer` / `io.ReadWriter` split at the signature
  instead of storing an abstract dependency.
* Controllers/services are created ad-hoc where needed — never stored as struct fields.

**RULE-STRUCT-004** — Composition `[review]`

* Embed instead of copying fields between related structs.
* Types are declared flat at package level — no type declarations inside functions.

**RULE-STRUCT-005** — Logic placement `[review]`

* Orchestration lives on the type that owns the unexported helpers it calls — never split a flow
  across a coordinator type and a helper type.
* Domain logic lives in the domain package, not in transport/CLI layers.

**RULE-STRUCT-006** — No anonymous struct types `[review]`

* Anonymous (inline) struct types are banned in production code: no anonymous
  struct literals as function arguments or template data, and never a loop over
  a slice of anonymous structs — unroll into named-helper calls instead.
* Any proposed use of an anonymous struct is a hot item: it goes into the plan
  for explicit approval (see hot-items.md).

**RULE-TYPE-001** — Typed models over generic containers `[review]`

* Never `json.RawMessage`, `any`, or `map[string]string` where a typed model exists or can be declared.
* Never duplicate an existing generic model as a local struct — reuse the model.

---

## FUNCTIONS, POINTERS & CONTEXT

**RULE-FUNC-001** — Signatures `[review]`

* `ctx context.Context` MUST be the first parameter (named `ctx`).
* `error` MUST be the last return value.
* No more than 3 return values including `error`; 3 (2 values + error) is the exception, not the norm.

**RULE-FUNC-002** — No one-line wrappers `[review]`

* A function whose body is a single call to another function adds a name, not behavior — inline it
  at the call site. Wrappers survive only with ≥2 callers AND added logic (defaulting, adaptation).
* Exemption: transport-level response helpers (e.g. a `respond` package wrapping
  `http.Error` per status code) are sanctioned despite one-line bodies once
  they have 3+ callers — the name carries the status-code contract.

**RULE-POINTER-001** — Pass by pointer `[review]`

* Structs MUST be passed by pointer. Passing by value is allowed only for small immutable value
  objects or when explicit copy semantics are required. If in doubt, use a pointer.

**RULE-POINTER-002** — Zero values over pointers `[review]`

* Express absence with the type's zero value where the zero value is unambiguous. Pointer fields
  only when nil-ness carries meaning the zero value cannot (unset vs. explicitly zero) or the
  persistence layer requires it.

**RULE-CTX-001** — Context propagation `[review]`

* `context.Context` MUST be passed through the call chain and MUST NOT be stored in long-lived
  structs (services, clients, repositories).
* Exception: it MAY be stored in short-lived, request-scoped structs (forms, request models) that do
  not outlive the request lifecycle.

**RULE-CTX-002** — Stored context `[review]`

* If `context.Context` is stored, the struct MUST be request-scoped and MUST NOT be reused across
  requests.

---

## CONTROL FLOW

**RULE-NEST-001** — Maximum nesting `[review]`

* A function body nests at most 2 levels of control flow (if/for/switch).
  Deeper logic is extracted into a named helper per level ("never-nester").
* Early returns are the primary de-nesting tool (extends RULE-ERR-002).

**RULE-COND-001** — Condition complexity `[review]`

* An `if` condition holds at most one binary boolean operator; never two `&&`.
  Compound logic is assigned to named predicate variables
  (`matchesEnabledRow := !hasEnabled || enabled`) or extracted into a helper.
* Refrain from `else if` generally — prefer `switch`, early returns, or
  sequenced named predicates. An `if x { … } else if y { … } else { … }` chain
  over two data sources is restructured into a direction-resolving helper.

---

## CALLS & LITERALS

**RULE-CALL-001** — When to go multiline `[review]`

A call MUST be multiline if ANY of the following holds: 5 or more arguments; any argument is a nested
call; single-line readability suffers.

**RULE-CALL-002** — Multiline format `[review]`

```go
FunctionName(
    param1,
    param2,
    param3,
)
```

* One argument per line; trailing comma required; closing parenthesis on its own line.
* gofmt does NOT decide whether a call is multiline (that is RULE-CALL-001) and does NOT split
  arguments one-per-line — it only normalizes indentation and the trailing comma once the call is
  already broken across lines. Treat the layout itself as a manual check.

**RULE-CALL-003** — Alphabetical ordering `[review]`

* Named fields in struct literals MUST be sorted alphabetically. Exception: `Id` is always first.
* Parameters in a function signature MUST be sorted alphabetically. Exceptions: `ctx` is always
  first, and a variadic / functional-options parameter (`opts ...Option`) is always last — Go
  requires the variadic parameter last, so it overrides alphabetical order.
* Not enforced by gofmt — verify in review.

**RULE-CALL-004** — Chunked slice consumption `[review]`

* When consuming a slice in chunks, advance the cursor by `len(chunk)` — never by the nominal chunk
  size constant, which drops the short final chunk or double-reads on partial consumption.

**RULE-CALL-005** — No composite literal as call argument `[review]`

* A composite literal (struct / slice / map literal) MUST NOT appear inline as
  a function-call argument. Assign it to a descriptively named variable first,
  then pass the variable (mirror of RULE-RETURN-001).

---

## RETURNS

**RULE-RETURN-001** — No composite literal in a multi-value return `[review]`

* A `return` that yields more than one value MUST NOT inline a composite literal (struct / slice /
  map / array literal). Assign the literal to a descriptively named variable first, then return the
  variable, so the trailing values (`nil` / `err` / …) are not lost behind a multi-line literal.
* A single-value return of a literal (`return &Foo{...}`) is fine — there is no trailing value to obscure.

```go
// BAD: the `, nil` hides behind the literal
func NewSesSender(...) (*SesSender, error) {
    return &SesSender{
        client:      sesv2.New(awsSession),
        fromAddress: fromAddress,
    }, nil
}

// GOOD: literal named, return is scannable
func NewSesSender(...) (*SesSender, error) {
    sender := &SesSender{
        client:      sesv2.New(awsSession),
        fromAddress: fromAddress,
    }
    return sender, nil
}
```

---

## ERROR HANDLING

**RULE-ERR-001** — Wrap with context `[review]`

* An error returned to a caller MUST be wrapped with `errors.Wrap` / `errors.Wrapf`, adding a
  `Component.Method: context` message (see RULE-LOG-001 for the prefix shape). New errors use
  `errors.New` / `errors.Errorf` with the same prefix.
* Bare `return err` is allowed ONLY as a direct pass-through where the callee's error is already
  fully contextualized and this function has nothing to add — do NOT double-wrap.
* Error message strings MUST NOT end with a period.

**RULE-ERR-002** — Early return `[review]`

* Handle errors with early returns; do NOT nest code unnecessarily after a check.
* Insert a blank line after an `if err != nil { … }` block when code follows. No blank line is
  required before a guard-clause `return`.
* The blank-line requirement applies after EVERY early-return guard block
  (`http.Error` + `return`, `continue` guards), not only `if err != nil`.

```go
if err != nil {
    return errors.Wrap(err, "Service.Method: Failed to load user")
}
```

**RULE-ERR-003** — Error variable `[review]`

* The error variable MUST be named `err` (not `e`, `error`, `err1`).
* In transaction closures, shadow the outer `err` — do NOT introduce `txErr` or similar.

**RULE-ERR-004** — Constant vs formatted messages `[review]`

* A constant message (no format directives) MUST use the non-formatting constructor: `errors.New`
  (new error) or `errors.Wrap` (wrapping). The formatting variants `errors.Errorf` / `errors.Wrapf`
  MUST be used ONLY when the message interpolates a value (i.e. the format string contains at least
  one verb such as `%s` / `%d`).

```go
// BAD: formatting variant with a constant string
return errors.Errorf("Model.Validate: Missing field Foo")
return errors.Wrapf(err, "Service.Method: Failed to load user")

// GOOD
return errors.New("Model.Validate: Missing field Foo")
return errors.Wrap(err, "Service.Method: Failed to load user")

// GOOD: formatting variant justified — the message interpolates a value
return errors.Errorf("Model.Validate: Invalid field Foo: %s", c.Foo)
return errors.Wrapf(err, "Service.Method: Failed to load user %s", userId)
```

---

## LOGGING

**RULE-LOG-001** — Format `[review]`

Log messages MUST follow: `<Receiver>.<Method>: <Context>: <Message>`. For a package-level function
(no receiver) the prefix is just the function name: `<Function>: <Context>: <Message>`.

The `<Message>` MUST begin with a capital letter (`Failed to load user`, `Missing field Foo`,
`Unsupported hash format`). This applies equally to error strings (RULE-ERR-001).

```go
log.Infof("RecordingService.GetLiveTvRecordingById: Recording %s: Fetching recording", recording.Id)
log.Errorf("IngestLiveTvRequestEventPubSubTask.Run: Event %s: Failed to save", event.Id)
// package-level function — function name only, no receiver
return errors.New("VerifyPassword: Unsupported hash format")
```

**RULE-LOG-002** — Content `[review]`

* Always include the component (`Receiver.Method`) and relevant identifiers (ids, request, …).
* Messages MUST be concise and descriptive.

---

## VALIDATION

**RULE-VALIDATE-001** — Model interface `[review]`

Models SHOULD implement:

```go
type Model interface {
    HasAutoIncrementId() bool
    Validate(skipId bool) error
}
```

**RULE-VALIDATE-002** — Field comments `[review]`

* Applies to EVERY `Validate()` func — model `Validate(skipId bool)`, API form
  `Validate(ctx, r, serverContext)`, and worker task validation alike.
* Each validated field MUST have a comment directly above its check, and the comment MUST name the
  value being validated (this is the sanctioned exception to RULE-COMMENT-001):
  * struct / payload fields → the Go field name (`// Email`, `// Otp`, `// DeviceName`)
  * request parameters → their source name: the header (`// CF-Device-ID`, `// X-Forwarded-For`,
    `// Origin`) or the path parameter (`// id`)

```go
// GameId
if c.GameId == "" {
    return errors.New("Game.Validate: Missing field GameId")
}

// CF-Device-ID
f.deviceUuid = r.Header.Get("CF-Device-ID")
if f.deviceUuid == "" {
    return errors.New("CreateBigScreenLoginIntentForm.Validate: Missing header CF-Device-ID")
}
```

**RULE-VALIDATE-003** — Optional fields `[review]`

* Missing optional values MUST NOT cause validation errors, BUT value constraints MUST still be
  checked when a value is present.

```go
// OptionalField
if c.OptionalField != "" && len(c.OptionalField) < 3 {
    return errors.New("Model.Validate: Invalid field OptionalField")
}
```

**RULE-VALIDATE-004** — Processing steps & order `[review]`

Applies to API form `Validate()` methods, which read whole requests rather than a single struct.

* Every non-trivial processing step — loading a related entity, trimming / normalizing input,
  hashing, or deriving a value — MUST have a short comment above it describing what it achieves
  (`// Resolve the calling app from its client id`, `// Load the referenced login intent`,
  `// Reject blacklisted email addresses`). One line only (this is covered by RULE-COMMENT-002).
* Steps SHOULD appear in this order so every form reads top-to-bottom like the same story:
  1. `nil` receiver guard
  2. authenticate / resolve the calling app
  3. request-context headers (device id, client ip, origin)
  4. path parameters
  5. read the JSON body (`ReadJSONForm`)
  6. validate the payload fields
  7. fetch / derive dependent models

```go
func (f *CreateLoginIntentForm) Validate(ctx context.Context, r *http.Request, serverContext *cfapi.Context) error {
    if f == nil {
        return errors.New("CreateLoginIntentForm.Validate: Called on nil")
    }

    // Resolve the calling app from its client id
    appId, err := validateAppByClientId(r, serverContext)
    if err != nil {
        return errors.Wrap(err, "CreateLoginIntentForm.Validate")
    }
    f.appId = appId

    // X-Forwarded-For
    f.clientIp = cfapiutil.ClientIpForRequest(r, "X-Forwarded-For")
    if f.clientIp == "" {
        return errors.New("CreateLoginIntentForm.Validate: Missing header X-Forwarded-For")
    }

    // Read the request payload
    if err := cfrequest.ReadJSONForm(r, f); err != nil {
        return errors.Wrap(err, "CreateLoginIntentForm.Validate: Failed to read JSON")
    }

    // Email
    f.Email = strings.ToLower(strings.TrimSpace(f.Email))
    if f.Email == "" {
        return errors.New("CreateLoginIntentForm.Validate: Missing field Email")
    }

    // Reject blacklisted email addresses
    isBlacklisted, err := serverContext.Db.Controller().Accounts.IsEmailBlacklisted(f.Email)
    if err != nil {
        return errors.Wrap(err, "CreateLoginIntentForm.Validate: Failed to check blacklist")
    }
    if isBlacklisted {
        return errors.New("CreateLoginIntentForm.Validate: Email is blacklisted")
    }

    return nil
}
```

**RULE-VALIDATE-005** — Validate vs IsValid `[review]`

* `Validate() error` belongs to models and forms implementing the validation contract. Predicate
  checks anywhere else are named `IsValid...() bool` — do not mint `Validate()` methods on
  non-model types.

---

## COMMENTS

**RULE-COMMENT-001** — Minimize `[review]`

* Avoid comments. Add one ONLY when intent is not obvious from names and code, or to record a
  non-obvious assumption the code cannot express.

```go
// BAD: restates what the code already says
// OtpSendClaim claims the send identified by sendCount by flipping the state to requires_action.
func (i *LoginIntent) OtpSendClaim(sendCount int) bool {

// GOOD: no comment — name and body carry the intent
func (i *LoginIntent) OtpSendClaim(sendCount int) bool {

// GOOD: documents a non-obvious assumption the code cannot express
// Django marks users without a usable password with the hash '!'.
func VerifyPassword(password, encoded string) bool {
```

**RULE-COMMENT-002** — Allowed groupings `[review]`

* Grouping / readability comments are allowed in tests and in `Validate` methods.

```go
// GOOD: grouping comments in a test
// first-send
test := &testCase{...}
tests = append(tests, test)

// resend
test = &testCase{...}
tests = append(tests, test)
```

---

## CONSTANTS

**RULE-CONST-001** — No magic values `[review]`

* Extract magic values into named constants: `const MaxRetries = 3`.

---

## IMPORTS

**RULE-IMPORT-001** — Formatting `[goimports]`

* Format imports with `goimports`.
* Separate internal packages using `goimports -local`.

---

## CLI (cobra)

**RULE-CLI-001** — Command wiring `[review]`

* Subcommands are wired with `AddCommand` in the root/main setup — never registered via `init()`
  side effects. At most one `init()` per package, and none for command registration.
* No package-level mutable flag variables — bind flags to fields on the command's own options struct.

---

## SUB-GUIDES (equally mandatory)

* Tests → `go-tests.md` (`RULE-TEST-*`, `RULE-DATA-*`, `RULE-ASSERT-*`)
* Goroutines → `go-goroutines.md` (`RULE-GR-*`)
* SQL → `../sql/sql.md` (`RULE-SQL-*`)

---

## ENFORCEMENT

* All rules are mandatory and applied on every change.
* Auto-format first: `gofmt` (indentation, trailing commas) and `goimports` (RULE-IMPORT-001); `go vet` MUST pass.
* Every `[review]` rule is a manual check; cite the `RULE-*` id when flagging a violation.
* Fix violations — do not merely note them.
* Tooling: `go mod vendor` regenerates `vendor/` from the module cache and wipes manual edits to
  vendored dependencies — never hand-edit `vendor/`; fix upstream or vendor a fork.

---

## QUICK CHECKLIST (review pass)

* [ ] Names: Pascal/camel, no stray underscores, acronyms per RULE-NAME-002 (`Otp`/`Id`/`Uuid`/`Db`/`Json`…), verbose, no boolean operator in method names (RULE-NAME-004), booleans as predicates
* [ ] File/type order: per-type blocks (type → its `New` → its methods), types alphabetical, private before public, free funcs last; struct fields private-then-public, alphabetical, `Id` first
* [ ] Signatures: `ctx` first, `error` last, ≤3 returns, params alphabetical
* [ ] Calls: multiline when required; struct-literal fields alphabetical (`Id` first)
* [ ] Returns: no composite literal in a multi-value return — name it, then return (RULE-RETURN-001)
* [ ] Errors: wrapped with `Component.Method:` context, no trailing period, named `err`, early return, tx shadowing; constant messages use `New`/`Wrap` not `Errorf`/`Wrapf` (RULE-ERR-004)
* [ ] Logging: `Receiver.Method: Context: Message`, message starts capitalized
* [ ] Validation: field comments present; optional-field handling correct
* [ ] Structure: no interface-typed struct fields (RULE-STRUCT-003); typed models over RawMessage/any/maps; embedding over field-copying; no one-line wrappers; zero values over pointers
* [ ] Comments minimal; constants for magic values; imports goimports-clean
* [ ] Sub-guides satisfied (tests, goroutines, SQL)
