# Değişiklik Günlüğü (CHANGELOG)

Bu proje [Keep a Changelog](https://keepachangelog.com/tr/1.0.0/) biçimini ve SemVer sürümlemeyi izler.

## [0.1.0] - 2026-11-16

### Eklenenler
- **Parabolik SVG Ray Sistemi:** Merkez `main` omurgası üzerinde `origin/main` ↑/↓ farkı, üst/alt yarı-küre ayrımı, Viyadük S-Ray ve Saf Parabol geometri modları ile deterministik Interval Lane Packing (`internal/gitengine/lanes.go`).
- **Birleşik Dal Kapsülleri (Branch Pair Capsules):** Üst katmanda PR, Review ve CI durumu; alt katmanda yerel dal adı, `HEAD` ve uncommitted dosya sayacı.
- **Akıllı Teşhis Motoru:** Çalışma dizinine dokunmadan `git merge-tree --write-tree` çakışma tespiti, `CHANGELOG.md` / lockfile için `Auto-Resolve`, Cross-PR dosya çarpışma radarı ve gürültüsüz Go test/panic CI log ayıklayıcı.
- **KeyLab Güvenlik & Embed Sunucusu:** `127.0.0.1` dinleme, `HttpOnly; SameSite=Strict` oturum çerezi, `X-Requested-With: gitarc` CSRF koruması, katı CSP ve `LocalFallbackEngine`.
