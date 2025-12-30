# go-rsc-boundary

A command-line tool that detects React Server Components (RSC) boundary violations and reports them in grep-compatible format.

Inspired by [boundary.nvim](https://github.com/Kenzo-Wada/boundary.nvim).

## Overview

When building applications with React Server Components, mixing server and client code can lead to subtle bugs. This tool scans your codebase to find where client components (those with `'use client'`) are being used, outputting results in a format that integrates seamlessly with editors and CI pipelines.

## Features

- Detects `'use client'` directive declarations
- Finds JSX usages of client components
- Outputs grep-compatible format (`filename:line:content`)
- Handles default, named, and aliased imports
- Resolves directory imports to index files
- Supports path aliases from tsconfig.json and jsconfig.json

## Installation

Install with Go:

```bash
go install github.com/conao3/go-rsc-boundary@latest
```

Build from source:

```bash
git clone https://github.com/conao3/go-rsc-boundary.git
cd go-rsc-boundary
go build
```

Try it without installing (requires Nix):

```bash
nix shell github:conao3/go-rsc-boundary
```

## Usage

Scan the current directory:

```bash
go-rsc-boundary
```

Scan a specific path:

```bash
go-rsc-boundary -path ./src
```

Enable verbose output:

```bash
go-rsc-boundary -v
```

## Output Format

Results are printed in grep format for easy integration with editors and tools:

```
path/to/file.tsx:15:      <Button />
path/to/file.tsx:20:      <Widget />
```

## Example

Given a client component:

```tsx
// components/Button.tsx
"use client"

export default function Button() {
  return <button>Click me</button>;
}
```

And a page that uses it:

```tsx
// app/page.tsx
import Button from '../components/Button';

export default function Page() {
  return (
    <div>
      <Button />
    </div>
  );
}
```

Running `go-rsc-boundary` outputs:

```
app/page.tsx:6:      <Button />
```

## Path Aliases

The tool automatically reads path aliases from your project configuration:

- tsconfig.json
- jsconfig.json
- tsconfig.base.json

Example configuration:

```json
{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["./*"],
      "@components/*": ["./components/*"]
    }
  }
}
```

## Configuration Defaults

| Setting | Default Value |
|---------|---------------|
| Directives | `'use client'`, `"use client"` |
| Extensions | .tsx, .ts, .jsx, .js |
| Max Read Bytes | 4096 |

## Skipped Directories

The following directories are automatically ignored during scans:

- node_modules
- .git
- dist
- build

## License

MIT
