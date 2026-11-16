---
name: gitarc-conflict-resolver
description: >-
  Diagnoses and resolves Git branch/PR merge conflicts in GitArc using the
  embedded gitarc MCP tools without touching the active working tree.
---

# GitArc Çakışma Çözüm İş Akışı

1. Önce `gitarc_check_conflicts` aracını çağır; çakışan dal kimliklerini (`branchId`), etkilenen dosyaları ve yalnızca çakışan pencereyi (`conflictHunks`, `unionPreview`, `oursPreview`, `theirsPreview`) oku.
2. Kullanıcı veya bağlam gereği en uygun stratejiyi seç:
   - `union`: `CHANGELOG.md`, kilit/liste dosyalarında her iki tarafı alt alta korumak için.
   - `ours`: Çakışan satırlarda `origin/main` güncel sürümünü baz almak için.
   - `theirs`: PR/özellik dalındaki kodu baskın kılmak için.
   - `custom`: İki tarafın mantığını birleştiren düzenlenmiş `customFiles` içeriğini göndermek için.
3. Ardından `gitarc_resolve_conflict` aracını `branchId`, `strategy` (ve gerekiyorsa `customFiles`) ile çağır. Araç izole `git worktree` içinde çözümü yazar, `go vet ./...` doğrulamasından geçirir, commit atar ve `origin` üzerine pushlar.
