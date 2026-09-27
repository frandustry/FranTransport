# ADR 0001: Use one Go module

Status: accepted

FranTransport V1 uses one repository and one Go module. The architecture is
still changing, and atomic refactors plus one test command are more valuable
than independent package release cycles. Package visibility and import rules
provide the required boundaries. Separate modules or repositories can be
considered only when independent versioning becomes a demonstrated need.
