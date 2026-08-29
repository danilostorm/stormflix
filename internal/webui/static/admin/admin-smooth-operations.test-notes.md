# Smooth Admin operations acceptance cases

- Scan-all polling with unchanged library/source structure updates cards in place instead of replacing the Bibliotecas page.
- A real library/source structural edit still falls back to the normal full renderer.
- Metadados & Capas renders normally on first open, then updates summary counts/job progress/actions in place.
- `Buscar em todas` processes enabled video libraries sequentially with `refresh=false`, preserving already matched titles.
- `Atualizar todas` processes enabled video libraries sequentially with `refresh=true`.
- Existing individual metadata/error/refresh buttons remain available.