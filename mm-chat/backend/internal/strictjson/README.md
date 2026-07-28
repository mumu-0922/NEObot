# strictjson

`strictjson` is the shared decoder for versioned Provider JSON that crosses a
security or persistence boundary. It rejects ambiguous JSON before a caller
validates domain fields.

## Responsibilities

- enforce a caller-provided byte limit;
- reject duplicate object keys at every nesting level;
- reject unknown Go struct fields and trailing JSON values;
- let custom `UnmarshalJSON` implementations require an exact field set.

It does not validate schemas, UUIDs, enums, confidence, ownership, or database
authority. Callers must perform those checks after decoding.

## Usage

```go
var output plannerOutput
if err := strictjson.Decode(body, 16*1024, &output); err != nil {
    return err
}
```

For an object with nullable-but-required fields, call `RequireExactKeys` from
its custom `UnmarshalJSON` implementation. The outer `Decode` call remains
mandatory because it owns recursive duplicate-key rejection.

## API

| Function | Contract |
| --- | --- |
| `Decode(body, maxBytes, target)` | Decode exactly one bounded JSON value with duplicate and unknown fields denied. |
| `RequireExactKeys(data, required)` | Require one object to contain exactly the named keys, including keys whose value is `null`. |

Current callers are the Memory worker extraction/decision parser and the
direct-user Memory action planner.

See [DESIGN.md](DESIGN.md).
