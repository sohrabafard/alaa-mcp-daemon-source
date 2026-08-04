# Validation

The v1 release gate is Windows-native. Run it from the project directory:

```powershell
pwsh -File .\scripts\verify.ps1
```

Exit `0` means requested checks passed; `1` means a check failed; `2` means a required tool, runtime, or requested side-effecting gate was unavailable. Exit `2` is not a pass.

Read [automated Windows gate](./validation/10-automated-windows-gate.md) for its exact checks and [manual acceptance](./validation/20-manual-acceptance.md) for observed runtime evidence. The non-Windows script is useful for platform-neutral orchestration but does not replace the Windows gate.
