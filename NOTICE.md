# Notice

kandev-plugin-azure-devops
Copyright (c) 2026 JnManso

This program is free software: you can redistribute it and/or modify it under
the terms of the GNU Affero General Public License, version 3 only, as
published by the Free Software Foundation. See [LICENSE](LICENSE).

## Third-party portions

### kandev-plugin-template (MIT)

The build, package, and release tooling, and `internal/sourcecontrol` and
`ui/src/source-control`, derive from
[kdlbs/kandev-plugin-template](https://github.com/kdlbs/kandev-plugin-template),
distributed under the following license:

```text
MIT License

Copyright (c) 2026 kandev

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### Kandev built-in Azure DevOps integration (AGPL-3.0-only)

`internal/azure` ports the REST client, models, and tests of the built-in
Azure DevOps integration from
[kdlbs/kandev](https://github.com/kdlbs/kandev) (`apps/backend/internal/azuredevops`
at commit `56af766dd9e9571e00e3c948426d75bf1c88b5a9`), distributed under the
GNU Affero General Public License, version 3 only. Copyright remains with
their authors.

### Kandev plugin SDKs (AGPL-3.0-only)

The plugin links `github.com/kdlbs/kandev/pluginsdk` and uses types from
`@kdlbs/plugin-sdk`, both from [kdlbs/kandev](https://github.com/kdlbs/kandev)
under the GNU Affero General Public License, version 3 only.

### Azure DevOps icon

`assets/icon.svg` is a Microsoft trademark. See
[assets/NOTICE.md](assets/NOTICE.md).
