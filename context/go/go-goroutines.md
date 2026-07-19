# GO GOROUTINE STYLE RULES

## BASELINE

Concurrent code MUST be readable top-to-bottom by a human who has never seen it before.

---

## WORKER POOL

RULE-GR-001:

* Concurrent batch processing MUST use a fixed worker pool: N goroutines consuming from a work channel
* MUST NOT spawn one goroutine per work item
* MUST NOT use channel-based semaphores (`make(chan struct{}, N)`) to limit concurrency

RULE-GR-002:

* Use `wg.Go` (Go 1.26) instead of manual `wg.Add` / `wg.Done`

---

## READING ORDER

RULE-GR-003:

* Code MUST read in data-flow order: input → processing → output
* The work source (what is being processed) MUST appear before the consumer (goroutines that process it)
* A reader scanning top-to-bottom MUST NOT encounter a channel consumer before understanding what the channel carries

BAD — consumer before producer:

```go
workChan := make(chan work)

for range workerCount {
    wg.Go(func() {
        for w := range workChan { // what is in workChan? unknown at this point
            process(w)
        }
    })
}

for _, item := range items { // NOW I see the data — too late
    workChan <- item
}
```

GOOD — data flow reads top-to-bottom:

```go
workChan := make(chan work, len(items))
for _, item := range items {
    workChan <- item
}
close(workChan)

resultChan := make(chan result, len(items))

for range workerCount {
    wg.Go(func() {
        for w := range workChan {
            resultChan <- process(w)
        }
    })
}

wg.Wait()
close(resultChan)

var results []result
for r := range resultChan {
    results = append(results, r)
}
```

---

## RESULT COLLECTION

RULE-GR-004:

* Worker results MUST be sent to a result channel
* MUST NOT write into a shared slice by index (`results[idx] = ...`)
* After all workers complete, close the result channel and collect into a slice

WHY: Index-based collection forces bookkeeping (passing indices through the pipeline, `batchWork.idx`, `results[work.idx]`). A result channel eliminates this: workers send, collector receives, done.

---

## WORKER BODY

RULE-GR-005:

* The `wg.Go` closure MUST contain only the range loop and a single function call per item
* The per-item work logic MUST be a named function

BAD — fat inline closure:

```go
wg.Go(func() {
    for w := range workChan {
        details, _, err := hub.GetNotificationDetails(w.id)
        if err != nil {
            log.Printf(...)
            resultChan <- errorResult(w.id, err)
        } else {
            log.Printf(...)
            resultChan <- successResult(w.id, details)
        }
    }
})
```

GOOD — named function:

```go
wg.Go(func() {
    for w := range workChan {
        resultChan <- fetchTelemetry(hub, w)
    }
})
```

---

## CHANNEL SIZING

RULE-GR-006:

* Work channels for pre-known input: buffer to `len(items)`, fill and close before starting workers
* Result channels: buffer to expected result count to prevent worker blocking
* Unbuffered channels: only when synchronization between sender and receiver is the intent

---

## SHARED STATE

RULE-GR-007:

* MUST NOT use `sync.Map` — use a `sync.Mutex` (or `RWMutex`) guarding a typed map
* Declare the mutex directly above the map it guards on the owning struct

---

## LOOP VARIABLES

RULE-GR-008:

* Go 1.22+ loop variables are per-iteration — MUST NOT pass the loop variable as an extra closure
  parameter or re-declare it (`w := w`) to "fix" capture; that idiom is dead weight

---

## ENFORCEMENT

* These rules MUST be applied strictly
* Fix violations instead of discussing them
