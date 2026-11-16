# GitArc — Parabolik Git & PR Yaşam Döngüsü Kokpiti

**GitArc**, [KeyLab (`sshpocket`)](https://github.com/syhgorath/keylab) tasarım ve güvenlik DNA'sını paylaşan bağımsız kardeş masaüstü/yerel web uygulamasıdır. Yerel Git dallarını (`HEAD`, uncommitted dosyalar, `origin` farkları) ile GitHub Remote, Pull Request, Review ve GitHub Actions CI durumlarını tek bir parabolik SVG ray tuvalinde birleştirir.

## Mimari Özellikler

- **Sıfır Harici Bağımlılık:** Go standart kütüphanesi (`net/http`, `os/exec`, `embed`) ve saf HTML5 + modern CSS + Vanilla JS + dinamik `<svg>`.
- **KeyLab Güvenlik Modeli:** Yalnızca `127.0.0.1` dinlemesi, `HttpOnly; SameSite=Strict` oturum çerezi, `X-Requested-With: gitarc` CSRF koruması, DNS rebinding kontrolü ve katı `Content-Security-Policy` (`style-src-attr 'none'`).
- **Parabolik SVG Ray Sistemi:**
  - **Üst Yarı-Küre:** GitHub Remote / Açık PR / CI rayları
  - **Merkez Omurga (`main`):** Commit düğümleri, etiketler ve doğrudan omurga üzerinde `origin/main ↑ / ↓` senkronizasyon hapı
  - **Alt Yarı-Küre:** Henüz pushlanmamış yerel çalışma dalları
  - **Viyadük S-Ray & Saf Parabol** geometri modları + deterministik **Interval Lane Packing** (`internal/gitengine/lanes.go`)
- **Akıllı Teşhis Motoru:**
  - Çalışma dizinine dokunmadan `git merge-tree --write-tree` ile çakışma tespiti ve `CHANGELOG.md` / `components.lock` / `go.mod` için `Auto-Resolve`
  - Açık PR'lar arası ortak dosya kesişimi (**Cross-PR Çarpışma Radarı**)
  - Regex tabanlı gürültüsüz CI hata ayıklayıcı (`--- FAIL:`, `panic:`, `dosya.go:satır`) ve yerel repro komutu

## Çalıştırma ve Test

```bash
# Birim ve CSP güvenlik testleri
go vet ./...
go test -v ./...

# Tek ikili olarak derle ve başlat
go build -o gitarc .
./gitarc

# Belirli bir yerel Git deposunu canlı modda aç
./gitarc -repo /Users/ziyayesilbahce/Developer/sshpocket
```

Arayüz, Go sunucusu olmadan `web/index.html` doğrudan tarayıcıda açıldığında da yerleşik `LocalFallbackEngine` ile simülasyon modunda çalışır.

## Yol Haritası (v0.2.0)
- Canlı modda gh run view --log-failed entegrasyonu
- Tek tıkla güvenli git fetch & prune --dry-run
