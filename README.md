# rsc-boundary

A command-line tool to detect and report React Server Components (RSC) `'use client'` boundary violations in grep format.

Inspired by [boundary.nvim](https://github.com/Kenzo-Wada/boundary.nvim).

## Features

- Detects components that declare `'use client'`
- Finds JSX usages of client components
- Outputs in grep format (`filename:line:content`)
- Handles default / named / aliased imports
- Resolves directory imports to `index` files
- Supports path aliases from `tsconfig.json` / `jsconfig.json`

## Installation

```bash
go install github.com/Kenzo-Wada/rsc-boundary@latest
```

Or build from source:

```bash
git clone https://github.com/Kenzo-Wada/rsc-boundary.git
cd rsc-boundary
go build -o rsc-boundary
```

## Usage

Scan current directory:

```bash
rsc-boundary
```

Scan specific path:

```bash
rsc-boundary -path ./src
```

Verbose output:

```bash
rsc-boundary -v
```

## Output Format

The tool outputs in grep format, compatible with most editors and tools:

```
path/to/file.tsx:15:      <Button />
path/to/file.tsx:20:      <Widget />
```

Format: `filename:line:content`

## Example

Given the following files:

**components/Button.tsx**:
```tsx
"use client"

export default function Button() {
  return <button>Click me</button>;
}
```

**app/page.tsx**:
```tsx
import Button from '../components/Button';

export default function Page() {
  return (
    <div>
      <Button />
    </div>
  );
}
```

Running `rsc-boundary` will output:

```
app/page.tsx:6:      <Button />
```

## Configuration

The tool uses sensible defaults:

- **Directives**: `'use client'`, `"use client"`
- **Extensions**: `.tsx`, `.ts`, `.jsx`, `.js`
- **Max Read Bytes**: 4096 (for directive detection)

## Path Aliases

The tool automatically detects and resolves path aliases from:

- `tsconfig.json`
- `jsconfig.json`
- `tsconfig.base.json`

Example `tsconfig.json`:

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

## Skipped Directories

The following directories are automatically skipped:

- `node_modules`
- `.git`
- `dist`
- `build`

## License

MIT
