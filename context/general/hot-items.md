# Hot Items

High-risk implementation classes. Every planned implementation in one of these classes gets its example implementation written into the plan for explicit approval **before any code is written** (see the feature-design and feature-refactor skills).

This is the shared baseline. Projects extend it with their own classes in their context dir.

1. SQL with CTEs
2. Goroutines, channels, and locking
3. New interfaces or generic types
4. Migrations and generated formats — write the source the generator consumes; never freehand its output
5. Validation, transaction, and guard logic — including any change that weakens or removes one
6. Anonymous (inline) struct types — any use requires an approved example in the plan
