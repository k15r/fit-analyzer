# fit-analyzer

Decodes Garmin FIT files and outputs structured activity data as YAML or JSON — session metadata, laps, splits, running dynamics, workout info, and elevation profile.

## Build

```sh
go build -o fit-analyzer .
```

## Usage

```sh
fit-analyzer [--format yaml|json] <file.fit> [file2.fit ...]
```
