# Building

This project intentionally has no .NET, Visual Studio, Windows SDK, or Windows printer API dependency.

1. Obtain a portable Go distribution in a user-owned directory.
2. Open PowerShell as the normal user.
3. From this directory run:

```powershell
go build -trimpath -ldflags "-s -w" -o GepfradPrint.exe .\cmd\gepfradprint
```

4. Run `GepfradPrint.exe`.
5. Browse to `http://127.0.0.1:17842/`.

The application intentionally binds only to loopback for its UI.
