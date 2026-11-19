# GitArc Mimari ve Refaktör Takip İndeksi (Refactor Index)

Bu belge, GitArc projesinin kod tabanı sağlığını, modül sınırlarını, tespit edilen tekrarları ve refaktör ilerlemesini takip etmek amacıyla tutulur.

---

## 📊 1. Mevcut Kod Boyutları ve Durum Haritası

| Katman | Dosya | Satır | Sorumluluk | Durum / Hedef |
|---|---|---|---|---|
| **Web Frontend** | `web/app.js` | 494 | Ana orkestratör ve yaşam döngüsü | ✅ 3.470 satırdan 494 satıra düşürüldü |
| **Web Frontend** | `web/modules/conflict-studio.js` | ~160 | Çakışma Stüdyosu: önizleme, sekmeler, uygulama | ✅ `modals.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/project-modals.js` | ~100 | Proje ekle/kaldır, klasör seçimi | ✅ `modals.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/tour.js` | ~105 | Spotlight kokpit turu | ✅ `modals.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/mcp-modal.js` | ~30 | MCP yapılandırma metni ve modalı | ✅ `modals.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/diff-viewer.js` | ~155 | Diff görüntüleyici | ✅ `modals.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/scene.js` | ~330 | `renderSVG`: sahne sınırları (saf), yaylar, omurga, düğümler | ✅ `canvas.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/interactions.js` | ~140 | Kaydırma, yakınlaştırma, ray süzgeçleri (saf kurallar + olaylar) | ✅ `canvas.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/rails.js` | ~100 | Tuval üstü etiketler/çipler: `rayCounts` (tek sayaç kuralı), omurga seçici, Geçmiş ve Uzun Ömürlü çipleri | ✅ `canvas.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/capsule.js` | ~345 | Kapsül modelleri (saf) + DOM yazıcıları | ✅ `canvas.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/kpi.js` | ~70 | KPI çipleri (`buildKpiModel` saf + `renderKPIs`) | ✅ `canvas.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/topbar.js` | ~125 | Üst çubuk ve senkron rozeti (saf modeller + `renderTopbar`) | ✅ `canvas.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/filters.js` | ~60 | Süzgeç koşulları ve KPI sayıları için tek tablo (`FILTERS`) | ✅ `canvas.js`'ten ayrıldı, `upper` süzgeci düzeltildi |
| **Web Frontend** | `web/modules/layout.js` | ~200 | Saf yerleşim matematiği (omurga, şerit, yay geometrisi, `computeView`) | ✅ `canvas.js`'ten ayrıldı, tekrarlar birleşti |
| **Web Frontend** | `web/modules/inspector.js` | ~310 | Sağ panel orkestratörü + olay bağlama (`renderInspector`) | ✅ 1.103 satırdan indi |
| **Web Frontend** | `web/modules/inspector-model.js` | ~170 | Saf kararlar (`branchViewModel`, `classifyCheck`, varsayılanlar, öneri doğrulama) | ✅ `inspector.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/inspector-cards.js` | ~525 | Saf HTML kartları (17 kart) | ✅ `inspector.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/studio.js` | ~250 | Alt Teşhis Stüdyosu panelleri ve satır içi diff | ✅ `inspector.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/proposal.js` | ~80 | AI öneri kartı olayları | ✅ `inspector.js`'ten ayrıldı |
| **Web Frontend** | `web/modules/modals.js` | 327 | Çakışma stüdyosu, proje yöneticisi ve kokpit turu | ✅ Modüler ayrıldı |
| **Web Frontend** | `web/modules/dock.js` | 261 | PocketUI alt çekmece, radar ve log akışı | ✅ Modüler ayrıldı |
| **Web Frontend** | `web/modules/utils.js` | 215 | `esc`, `truncate`, zaman ve güvenlik yardımcıları | ✅ Modüler ayrıldı |
| **Web Frontend** | `web/modules/state.js` | 144 | Reaktif durum mağazası ve DOM kayıt defteri | ✅ Modüler ayrıldı |
| **Web Frontend** | `web/modules/api.js` | 25 | REST API istemcisi ve hata yönetimi | ✅ Modüler ayrıldı |
| **Web Frontend** | `web/modules/constants.js` | 16 | Layout ve geometri sabitleri | ✅ Modüler ayrıldı |
| **Web Frontend** | `web/fallback.js` | 672 | Tarayıcı çevrimdışı (offline) simülasyon motoru | ✅ İyi durumda (Tri-Mode parity korunuyor) |
| **Web Frontend** | `web/tasarim.css` | 2.931 | Tasarım sistemi, tema, PocketUI, animasyonlar | ✅ Düzenli, Strict CSP uyumlu |
| **Go Engine** | `internal/gitengine/simulation.go` | 1.423 | In-memory simülasyon repo yöneticisi | ✅ `simulation_enterprise_mock.go` (523 satır) ayrıldı |
| **Go Engine** | `internal/gitengine/simulation_enterprise_mock.go` | 523 | MegaCorp 50 geliştirici demo simülasyon veri yükü | ✅ Temiz ayrıldı |
| **Go Engine** | `internal/gitengine/collector.go` | 1.200 | Yerel Git komutları (`git log`, `merge-tree`, `for-each-ref`) | ✅ `collector_github.go` ayrıldı |
| **Go Engine** | `internal/gitengine/collector_github.go` | 334 | GitHub CLI/API metadata, PR listeleri ve ruleset kontrolleri | ✅ Temiz ayrıldı |
| **Go Engine** | `internal/gitengine/conflict_studio.go` | 714 | İzole 3-yollu çakışma stüdyosu ve motoru | ✅ İyi durumda |
| **Go Engine** | `internal/gitengine/live_mutations.go` | 584 | Canlı Git/GitHub mutasyonları | ✅ İyi durumda |
| **Go Engine** | `internal/gitengine/diagnostics.go` | 403 | Ters indeks Cross-PR ve CI log regex parser | ✅ İyi durumda |
| **Go Engine** | `internal/gitengine/proposals.go` | 280 | AI commit/PR teklif stüdyosu | ✅ İyi durumda |
| **Go Engine** | `internal/gitengine/guard_config.go` | 264 | Güvenlik kalkanı ve secret engelleme kuralları | ✅ İyi durumda |
| **Go Engine** | `internal/gitengine/projects.go` | 233 | Kalıcı proje yönetimi (`projects.json`) | ✅ İyi durumda |
| **Go Engine** | `internal/gitengine/lanes.go` | 99 | Interval packing şerit yerleşim algoritması | ✅ Kompakt ve odaklı |
| **Go MCP** | `internal/mcp/server.go` | 897 | Sıfır bağımlılıklı JSON-RPC 2.0 MCP sunucusu | ✅ Canlı `activeContext` entegreli |
| **Go HTTP/API** | `main.go` | 674 | HTTP router, güvenlik middleware'leri, CLI bayrakları | ✅ Temiz ve güvenli |

---

## 🔍 2. Tespit Edilen Tekrarlar (Code Duplications) ve İyileştirme Alanları

### A. Go Motoru Tarafı
1. **GitHub Metadata & Yerel Git Ayrımı (`collector.go`):**
   - Yerel Git komutları (`git log`, `git merge-tree`, `git for-each-ref`) ile harici GitHub CLI (`gh pr list`, `gh api`) kodları aynı dosyada iç içe geçmiş durumda.
   - **Çözüm:** GitHub sorguları `collector_github.go` dosyasına taşınacak.
2. **Enterprise Simülasyon Veri Yükü (`simulation.go`):**
   - 50 kişilik geliştirici kadrosu, 16 commit düğümü ve 12 dal mock verisi `simulation.go` dosyasının %27'sini kaplıyor.
   - **Çözüm:** `simulation_enterprise_mock.go` içine alınacak.
3. **Mükerrer CLI/Komut Çalıştırıcıları:**
   - `runCmd` fonksiyonu ve zaman aşımı kalıpları kontrol edilip standartlaştırılacak.

### B. JavaScript / Web Arayüzü Tarafı
1. **Monolitik `app.js` Yapısı:**
   - Tek dosyada 3.470 satır kod bulunuyor. SVG koordinat hesaplamaları, Inspector DOM oluşturucuları, modal kontrolleri ve REST istekleri iç içe.
2. **Kopya HTML Kart Şablonları:**
   - Eylem butonları (`🎯 Dala Geç`, `🧹 Sil / Temizle`, `🚀 Push & PR`) hem SVG içinde (`buildCapsuleForeignObject`), hem sağ Inspector'da (`renderInspector`), hem de Bottom Dock radar kartlarında benzer biçimde tekrar ediyor.
3. **Planlanan Modüler Yapı (Vanilla ES Modules - Sıfır Dış Bağımlılık):**
   - `web/modules/state.js` -> Global durum, reaktif seçimler, snapshot cache
   - `web/modules/api.js` -> `/api/*` REST uç noktaları ve hata yönetimi
   - `web/modules/canvas.js` -> SVG omurga, paraboller, bezier eğrileri, zoom/pan
   - `web/modules/inspector.js` -> Sağ panel (Inspector) kartları ve form işleyicileri
   - `web/modules/dock.js` -> PocketUI alt çekmece, radar ve log akışı
   - `web/modules/modals.js` -> Çakışma stüdyosu, proje yöneticisi ve kokpit turu
   - `web/modules/utils.js` -> `esc`, `truncate`, zaman formatlama, DOM yardımcıları
   - `web/app.js` -> Ana orkestratör (~150 satır)

---

## 📋 3. Refaktör İlerleme Takip Listesi

- [x] **Adım 0:** Refaktör Envanter İndeksinin Kurulması (`REFACTOR_INDEX.md`)
- [x] **Adım 1:** Go - `simulation.go` içinden `buildEnterpriseSimulation` -> `simulation_enterprise_mock.go` ayrımı
- [x] **Adım 2:** Go - `collector.go` içinden GitHub CLI/API fonksiyonları -> `collector_github.go` ayrımı
- [x] **Adım 3:** Go testlerinin ve binary derlemesinin doğrulanması (`go test -v ./... && go build`) (25/25 PASS)
- [x] **Adım 4:** JS - `web/modules/utils.js`, `constants.js`, `state.js` ve `api.js` altyapısının kurulması
- [x] **Adım 5:** JS - `web/modules/canvas.js` (SVG render motoru) ayrımı
- [x] **Adım 6:** JS - `web/modules/inspector.js` ve `web/modules/dock.js` ayrımı
- [x] **Adım 7:** JS - `web/modules/modals.js` ayrımı
- [x] **Adım 8:** JS - Üst/Alt Ray Popover (`renderRayPopovers`) ve etkileşim entegrasyonu tamamlandı
- [x] **Adım 9:** `TestWebAssetsStrictCSPCompliance`, `node --check` ve tam sistem entegrasyon doğrulaması (25/25 PASS)
