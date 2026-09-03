# Runtime date prompt fix

- Removes `get_current_time` from the default tool registry and deletes its implementation.
- Injects host date, year, local time, timezone and UTC offset into every system prompt at request time.
- Marks the injected clock data as authoritative runtime facts so the model does not fall back to stale training-era dates.
- Adds regression tests for date injection and for the absence of the old time tool.
- Updates migration parity documentation.
