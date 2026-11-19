# GitArc Teknik Devir Teslim ve Mühendislik Rehberi (`HANDOVER.md`)

> **Mevcut Sürüm:** `0.10.0` (`VERSION` ve `internal/gitengine/collector.go` `AppVersion`)  
> **Modül / Go Sürümü:** `module gitarc` · `go 1.27.3` · Sıfır dış bağımlılık (`go.mod`)  
> **Son Doğrulanmış Test Durumu:** `go vet ./... && go test -count=1 -race -cover ./... && go build -o gitarc .` → **166 üst düzey Go testi + 213 JS testi geçti** (`gitarc`: 21 (JS sarmalayıcı dahil; `main()` ve OS araçları dahil kapsam **%74**), `internal/gitengine`: 106 · kapsam **%84**, `internal/mcp`: 3, `internal/model`: test dosyası yok), `node --check web/*.js web/modules/*.js` → **0 hata**. Ayrıntılar için bkz. **Bölüm 8 (Test Altyapısı)**.

---

## 1. Proje Özeti ve Görsel Parabolik Metafor

GitArc, standart düz `git log` ağaçları yerine Git ve GitHub yaşam döngüsünü **parabolik ray topolojisi** ile gösteren, sıfır bağımlılıklı bir Go + Vanilla SVG/JS yerel masaüstü/ekip kokpitidir:

- **Merkez Yatay Ray (`main` / `defaultBranch`):** Üzerindeki commit düğümleri (`CommitNode`), tag'ler ve `origin/main` ile senkron farkı (`Ahead` / `Behind`).
- **Alt Yarı-Küre (`hemisphere: "lower"`, kesikli mor yay):** Henüz uzak sunucuya (`origin`) pushlanmamış ve PR açılmamış yerel dallar + kirli çalışma ağacını gösteren sanal `local-wip-working-tree` (`wip/working-tree`) yayı.
- **Üst Yarı-Küre (`hemisphere: "upper"`, mavi/kırmızı/turuncu/kehribar yay):** Uzak dala pushlanmış, açık Pull Request ve GitHub Actions CI durumunu taşıyan aktif dallar (yerelde kopyası silinmiş `origin/*` / GitHub-only PR'lar dahil).
- **Tam Kapanmış Parabol (`state: "merged"`, yeşil yay):** `baseSha` noktasından çıkıp `mergeSha` noktasında yeniden `main` omurgasına dönen birleşmiş dallar.
- **Akıllı Hata Radarı & Çakışma Kapsülleri:** Başarısız CI (`ci-fail`), merge çakışması (`conflict`), çalışan CI (`ci-running` nabız efekti), Cross-PR dosya çarpışması ve `origin: gone` kalıntıları üstünde eylem düğmeleri taşır.

---

## 2. Sürüm Kronolojisi (`v0.1.0` → `v0.10.0`)

| Sürüm | Öne Çıkan Özellikler |
| :--- | :--- |
| **v0.1.0 – v0.2.0** | Parabolik SVG motoru, Canlı Git + Simülasyon ikili motoru, `git merge-tree --write-tree` ile çalışma dizinine dokunmayan çakışma ve Cross-PR algılama, gürültüsüz CI log ayıklama, tek tıkla Merge/Squash/Rebase. |
| **v0.3.0** | Gömülü `stdio` JSON-RPC 2.0 MCP sunucusu (`./gitarc -mcp`), açık PR başlık/açıklama düzenleme (`gh pr edit`), `HEAD` son commit `--amend --only` düzenleme, `git switch` checkout ve Terminal komutu kutusu. |
| **v0.4.0** | Uzak (`origin/*` / GitHub-only PR) dal keşfi (`collectRemoteOnlyBranches`), remote-only çakışma algılama, `gh pr close --delete-branch` / `git push origin --delete` temizliği, gerçek bare/clone git deposu mutasyon testleri. |
| **v0.5.0** | **Çakışma Çözüm Stüdyosu** (`/api/conflict/inspect`): Sol (Olan 3-yollu diff hunks) / Sağ (Olacak canlı önizleme), 4 strateji (`union`, `ours`, `theirs`, `custom`), izole geçici `git worktree` içinde çözüm + `go vet ./...` güvenlik kilidi, Antigravity MCP bağlantısı (`gitarc_resolve_conflict`, `.agents/plugins/gitarc/`). |
| **v0.6.0** | Kalıcı `📜 Loglar` modalı (`CockpitSnapshot.Logs` + `/api/logs/clear`), stüdyo içi kalıcı kopyalanabilir hata kutusu (`#studio-error-banner`), dosya türüne göre `★ ÖNERİLEN` strateji danışmanı. |
| **v0.7.0** | **Mega-Repo (10K Geliştirici) Ölçekleme:** SHA-bazlı `evalCache` (`CacheHits`), `--sort=-committerdate --count=40` dal penceresi, $O(N \cdot F)$ ters indeks Cross-PR motoru, `👤 Benim Dallarım (@me)` ve `📂 Klasör/Dosya Merceği`, `-team` ekip sunucusu modu, Windows `rundll32` tarayıcı açılışı, Branch Protection (`/api/repo/protect`) ve Update Checker (`/api/system/update-check`). |
| **v0.8.0** | **PocketUI Alt Çekmece (Bottom Dock)** (Radar/Loglar/İşlemler), **Tam Genişlik Hata & Uyarı Kartları** (dosya etiketleri, CI hata logu snippet'i, kopyalanabilir komut satırı), **Taze Dal Ayrımı** (`🌱 Yeni Yerel Dal` · `aheadMain == 0` dalların yanlışlıkla `origin: gone` sayılmasını engelleme), **Mükerrer Dal Açma Kalkanı**, **Konsolsuz Commit & PR Stüdyosu** (`/api/working-tree/commit`, yerel `git checkout -b` akışı), **Push Öncesi PR Başlık & Açıklama** (`PromoteBranchWithMeta`), **`CI: RUNNING` Nabız Efekti** (`.running-pulse`), polling tabanlı **Otomatik Merge Kuyruğu**, **Alt Ray Yerel Kalıntı Kuyruğu** + `💻 Local` rozeti, otomatik `HEAD → main` geçişli `DeleteBranch`/`PruneGone`. |
| **v0.9.0** | **Koyu/Açık Tema Düğmesi** (`theme.js`), **okunabilir ölçek + sağ uca yaslanma** (`computeView`, enterprise demosu okunur), omurga genişliğinde yalnızca birleşmiş yayların sayılması, sınırsız şerit ve dinamik omurga yerleşimi, `main()`/`openBrowser`/klasör seçici dahil geniş test altyapısı (sahte `gh` stub'ı, JS testleri, `HOME` izolasyonu, `version_test.go`), diff görüntüleyici, AI teklif kartı, `.gitarc` güvenlik yapılandırması, README ekran görüntüleri. |
| **v0.10.0** | **Dürüst kokpit:** uydurma CI verileri kalktı, merge sonrası `main` CI rozeti ve radar uyarısı (`fetchMainCI`); **hız:** dal başına `gh` sorguları önbellekte (anlık görüntü ~13 sn → ~3 sn), Temizle tek `git push --delete` ile hepsini siler (`IsPrunable` tek kural); **birleşmiş PR detayı** (gerçek başlık/açıklama) ve dosya dosya diff'i (`merge` diff türü); **sol şerit kalktı**, köşe etiketleri/çipler, **Yerel/Uzak** adları; **omurga dalı seçici** (`SetBaseBranch`) ve `origin/HEAD` tespiti; **uzun ömürlü dallar** (`IsLongLivedBranch`); "uzağa gidiyor" izi; modüler arayüz (`canvas.js`/`inspector.js`/`modals.js` bölündü), açık tema düzeltmeleri |

---

## 3. Dosya ve Mimari Haritası

### Go Backend (`main.go`, `internal/`)
- **`main.go`:**
  - Bayraklar: `-addr` (`127.0.0.1:7432`), `-repo` (`.`, demo için `"sim"`), `-open` (`true`), `-mcp` (`false`), `-team` (`false`).
  - Statik dosyaları `//go:embed web` ile gömer ve kök yanıtta sıkı CSP başlığı set eder:
    `default-src 'none'; script-src 'self'; style-src 'self'; style-src-attr 'none'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`
  - **34 `/api/*` rota kaydı** (`a.guard(...)` arkasında; eski istemciler için `/api/sync-spine`, `/api/reset`, `/api/run-local`, `/api/branch/prune-gone` takma adları dahil):
    - `GET /api/snapshot`, `GET /api/diff` (`POST` da kabul eder), `GET /api/system/git-status-ping` (sıfır maliyetli değişiklik algılama imzası)
    - `POST /api/repo/switch`, `/api/repo/add`, `/api/repo/remove`, `/api/system/choose-folder`
    - `POST /api/branch/create`, `/api/branch/checkout`, `/api/branch/merge`, `/api/branch/promote`, `/api/working-tree/commit`, `/api/working-tree/discard`, `/api/working-tree/diff-stat`, `/api/proposal/approve`, `/api/proposal/dismiss`
    - `POST /api/conflict/inspect`, `/api/branch/resolve`, `/api/branch/fix-ci`
    - `POST /api/branch/delete`, `/api/branch/prune`, `/api/pr/edit`, `/api/commit/amend`
    - `POST /api/spine/sync`, `/api/logs/clear`, `/api/system/update-check`, `/api/repo/protect`, `/api/simulation/reset`, `/api/test/run`
  - `Cache-Control: no-cache, no-store, must-revalidate` (+ `Pragma`, `Expires`) tüm statik dosyalara `root()` üzerinden eklenir; bu yüzden `index.html` içinde **elle yazılı `?v=...` sürüm sorgusu kullanılmaz** (bkz. Bölüm 6, madde 7).
- **`internal/model/model.go`:** Frontend ve MCP'nin tükettiği tüm JSON veri kontratları (`CockpitSnapshot`, `RepoMeta`, `BranchArc`, `PRStatus`, `CIStatus`, `ConflictDetail`, `CrossPRWarning`, `ConflictStudioPayload`, `ActionLogEntry`).
- **`internal/gitengine/collector.go`:**
  - `AppVersion` sabiti (`VERSION` dosyası ile her zaman senkron tutulmalıdır!).
  - `CollectLiveRepo`: Varsayılan dal tespiti (`main`/`master`), `main` omurga commit düğümleri, `gh` CLI önbellekli PR/CI/BranchProtection sorgusu, çalışma ağacı kirli dosya analizi (`parsePorcelainFiles` + `buildSuggestedCommitAndPR`), `collectLocalBranches`, `collectRemoteOnlyBranches`, `evalCache` (SHA değişmemiş dallarda `merge-tree`/`rev-list` tekrar koşmaz).
- **`internal/gitengine/simulation.go`:**
  - `Manager`: Hem canlı git deposunu hem bellek-içi `simRepo` simülasyonunu yöneten tek giriş noktası.
  - İki zengin simülasyon deposu: `sim-sandbox` ("GitArc Sandbox (Temel Demo)") ve `sim-enterprise` ("MegaCorp Platform (50 Geliştirici · Enterprise Demo)" · 4 ana track: `develop`, `staging`, `release/v3.3`, `support/lts-2.x`, 50 farklı yazılımcı ismi, CI Running, CI Failure, Cross-PR Collision, Helm conflict, local remnant ve fresh local branch).
  - In-memory simülasyon yaşam döngüsü (`simWorkspace`, `reset`, sanal adımlar) ve kalıcı `actionLogs`.
- **`internal/gitengine/projects.go`:**
  - Proje Yönetimi: Yerel Git depolarını kaydetme, `~/.gitarc/projects.json` kalıcı dosyasında saklama, listeden kaldırma ve çalışma ağacındaki kirli değişiklikleri güvenle geri alma (`DiscardWorkingTree` / `git restore` + `git clean`).
- **`internal/gitengine/live_mutations.go`:**
  - Canlı Git ve GitHub mutasyonlarının tamamı (`commitWorkingTreeLive`, `promoteBranchLive`, `mergeBranchLive`, `deleteBranchLive`, `pruneGoneLive` otomatik `HEAD → main` geçişi, `updatePRMetaLive`, `amendLastCommitLive`, `createBranchLive`, `checkoutBranchLive`).
  - Her mutasyon öncesi `IsTeamMode()` ve güvenlik kontrolleri yapılır, sonuç `m.recordLog` / `m.logLiveOutcome` ile geçmişe yazılır.
- **`internal/gitengine/lanes.go`:** `AssignLanes` — her yarı-küre (`upper`/`lower`) için bağımsız, deterministik interval packing. Kısa aralıklar iç şeride (Lane 1) gider, eşitlikte `start` sonra `Name` sırası kullanılır; açık/gone dallar uç kapsülü için +1 tampon ayırır. **Şerit sayısı sınırsızdır** (eski `MaxHemisphereLanes = 4` kaldırıldı); `web/fallback.js` içindeki kopyası aynı davranışı korur (Tri-Mode eşliği).
- **Diğer `internal/gitengine` dosyaları:** `noproject_test.go` (`gitRoot`/`repoRootFor`, "proje yok" durumu, demo yalnızca `-repo sim`), `mainci_test.go` + `collector_github.go: fetchMainCI` (merge sonrası omurga CI: `RepoMeta.MainCI`, `SyncSteps` adımı `mainci`, radar `main_ci_failure`; gh stub anahtarı `run_list`), `prunable.go` (`IsPrunable`: 🧹 Temizle sayacı ve temizliği için TEK kural; `fallback.js` ile aynı), `basebranch.go` (omurga dalını elle sabitleme: `SetBaseBranch`, `~/.gitarc/base-branches.json`), `defaultbranch_test.go` (varsayılan dal tespiti), `longlived.go` (`IsLongLivedBranch`: develop/release/*/1.x gibi uzun ömürlü dal kuralı; `PruneGone`, `DeleteBranch` ve "kalıntı" tanılamasından korunur; kural `fallback.js` ile birebir aynı), `collector_github.go` (`gh` ile açık/birleşmiş PR, CI ve Branch Protection/Ruleset sorgusu, 10 sn `ghCache`; `InvalidateGHCache` her mutasyondan sonra çağrılır), `diagnostics.go` (CI log ayıklama, Cross-PR, `IsSpecialAutoMergeFile`), `diff_service.go`, `guard_config.go` (secret/`git add -A` kalkanı), `proposals.go` (AI commit/PR teklif IPC dosyası `.git/gitarc_proposal.json`), `logs_persistence.go` (`~/.gitarc/logs.json`), `playground.go`, `simulation_enterprise_mock.go`.
- **`internal/gitengine/conflict_studio.go`:** `InspectBranchConflict` ve `ResolveBranchConflict`. Çakışan dosyaları geçici `git worktree add --detach` üzerinde 3-yollu `git merge-file -p` ile inceler; çözümden sonra `go.mod` mevcutsa `go vet ./...` çalıştırır, hata alırsa `worktree`'yi temizleyip pushlamadan hatayı döner.
- **`internal/mcp/server.go`:** MCP JSON-RPC 2.0 `stdio` sunucusu (`protocolVersion = "2024-11-05"`). Sunulan 5 araç: `gitarc_status`, `gitarc_check_conflicts`, `gitarc_ci_failure_digest`, `gitarc_sync_main`, `gitarc_resolve_conflict`.

### Frontend (`web/`)
- **`web/index.html`:** Üst bar (repo seçici, `➕ Proje Ekle`, `🎓 Tur`, KPI çipleri, arama/filtre/mercek, Loglar/MCP/Güvenlik&Güncelleme butonları), sol SVG parabol sahnesi (`#rail-svg`), tuvalin altında **PocketUI Tabbed Bottom Dock** (`#bottom-dock` · 🚨 Hata & Uyarı Radarı, 📜 Sistem & Git Logları, ⚡ Son İşlemler, daraltma/temizleme araçları), sağ Inspector (`#inspector-detail`) ve 7 modal/overlay:
  1. `#create-branch-modal`
  2. `#conflict-studio-modal`
  3. `#mcp-connect-modal`
  4. `#logs-modal`
  5. `#security-update-modal`
  6. `#add-project-modal`
  7. `#tour-backdrop` (Etkileşimli spotlight kokpit turu)
- **`web/tasarim.css`:** Tüm tema renkleri (`--sky`, `--purple`, `--emerald`, `--danger`, `--warn`, `--warn-orange`), SVG yay animasyonları (`ciStrokePulse`, `ciRunningPulse`), kart nabız halkaları (`pulseRedRing`, `pulseRunningRing`, `dotBlink`), **PocketUI Bottom Dock** (`.bottom-dock`, `.dock-collapsed`), **Tam Genişlik Hata Radarı Kartları** (`.radar-card`, `.rc-header`, `.rc-badge`, `.rc-files-row`, `.rc-ci-snippet`, `.rc-cmd-row`), ve modal/form düzenleri.
- **`web/fallback.js`:**
  - `LocalFallbackEngine`: Backend sunucusu olmadan (`file://` veya bağlantı kesintisinde) kokpiti bağımsız çalıştıran tarayıcı motoru (`deleteBranch` ve `pruneGone` dâhil yeşil yayları korur, simüle CI extraction, çakışma ve pipeline yönetimini barındırır). `app.js`'den önce yüklenir (`window.LocalFallbackEngine`).
- **`web/modules/*.js` (ES modülleri, `app.js` bunları içe aktarır):** `state.js` (reaktif durum + DOM kayıt defteri), `constants.js` (`MAIN_Y`, `SPINE_STEP_X`, `LANE_*` geometri sabitleri), `utils.js` (`esc`, `svgEl`, durum yardımcıları), `api.js`, `layout.js` (omurga/şerit/yay geometrisi/`computeView`, DOM'suz ve Node'da test edilir), `capsule.js` (dal kapsülleri; `whereModel` Uzak/Yerel satırı: karar mantığı saf modellerde `openCardModel`/`mergedPillModel`/`badgeModel`/`actionRowHtmlOf`, DOM yazıcıları ince; yeni bir kapsül rozeti/butonu eklerken önce modele ekle ve `tests/js/capsule.test.mjs`'e test yaz; kullanıcı verisini `innerHTML`'e basarken **her zaman `esc()`**), `kpi.js` (KPI çipleri; `buildKpiModel` saf), `topbar.js` (üst çubuk ve senkron rozeti; `syncBadgeModel`, `versionView` vb. saf), `filters.js` (süzgeç koşulları ve KPI sayıları için TEK tablo: `FILTERS`; yeni bir süzgeç eklerken yalnızca buraya ekle, çip sayısı otomatik uyar), `syncsteps.js` (veri toplama adımları ve koruma görünümü), `trace.js` ("uzağa gidiyor" izi: `detectPushes` iki snapshot arasında push'u bulur, `playPushTraces` kesik çizgi + SMIL noktası çizer; `applySnapshot` çağırır), `polling.js` (arka plan yoklamasının "şimdi tam snapshot çek" kararı: git değişikliği VEYA koşan CI; GitHub verisi yalnızca snapshot istenince yenilendiği için CI koşarken seçili olsa bile aralıklı çeker), `theme.js` (koyu/açık tema: `<html data-theme>` + `localStorage` anahtarı `gitarc_theme`; renkler `tasarim.css` içindeki `:root` / `:root[data-theme="light"]` değişken bloklarındadır), `scene.js` (`renderSVG`: sahne sınırları `computeSceneBounds`, yaylar, omurga, düğümler; `branchToneClass`/`branchStrokeColor`/`matchesFilter`), `interactions.js` (kaydırma, yakınlaştırma, ray süzgeçleri, boş alana tıklama; sayısal kurallar saf: `wheelZoom`, `panFromDrag`, `toggleFilterKey`), `rails.js` (tuval üstü etiketler ve çipler: sol üstte `Üst Ray` süzgeci + `⚙ <omurga>` seçici (`basePickerModel`) + `🗂 Geçmiş` + `⏳ Uzun Ömürlü` çipleri, sol altta `Alt Ray` süzgeci; `rayCounts` TEK sayaç kuralı; çipler birbirini kapatır: `closeChipPopovers`. Sol şerit KALKTI, tuval tam genişlik), `inspector.js` (sağ panel orkestratörü: hangi kartlar hangi sırayla, olay bağlama), `inspector-model.js` (saf kararlar: `branchViewModel`, `classifyCheck` — CI kontrolleri için tek sınıflandırma, `workingTreeDefaults`, `promoteDefaults`, `buildApprovePayload`), `inspector-cards.js` (saf HTML kartları; şablon boşlukları bilerek korunur), `studio.js` (alt Teşhis Stüdyosu panelleri ve satır içi diff), `proposal.js` (AI öneri olayları), `dock.js`, `conflict-studio.js` (Çakışma Stüdyosu), `project-modals.js` (proje ekle/kaldır), `tour.js` (kokpit turu), `mcp-modal.js` (MCP yapılandırma metni), `diff-viewer.js`. `modals.js` artık yoktur.
  - **`layout.js` (saf yerleşim matematiği; `scene.js` yalnızca çizer) — yerleşim mantığı:** `layoutSpinePositions` (omurga düğümleri dal kalabalığına göre açılır, hep artan sıra; **yalnızca birleşmiş yaylar kalabalığa sayılır**, açık dalların kapsülleri `layoutOpenApexPositions` ile ayrı yerleşir. Açık dallar da sayıldığında enterprise demosunda adımlar 1000 px'e şişiyordu), `layoutBranchCrowd` (birleşmiş dallar için `{crowd, rank}`), `yOfLane` (şerit sınırı yok), `computeBranchGeometry` (birleşmiş yaylarda dış şeritler daha yüksek ve geniş olur, sol marj `SPINE_START_X - 50`), `layoutOpenApexPositions` (açık kapsüller arası en az `OPEN_CAPSULE_MIN_GAP_X`). **`splitHistory` / `MAX_MERGE_SPREAD`:** `historyOnly` dallar çizime hiç girmez (`renderSVG` yalnızca `drawn` listesini kullanır) ve tuvalin sol üstündeki `#btn-history-lane` çipinde `renderHistoryCard` ile listelenir; birleşmiş yayların uç genişletmesi `MAX_MERGE_SPREAD` (2 omurga adımı) ile sınırlıdır, yoksa çok şeritli depolarda çizim gerçek merge konumundan kopar. **`computeView` (saf fonksiyon):** görünür `viewBox`'ı hesaplar. Sahne ekrana sığdırılınca ölçek `MIN_READABLE_SCALE` (0,5 px/birim) altına düşecekse tamamı sığdırılmaz; bu ölçekte çizilir, ilk açılışta ve depo değişince en güncel (sağ) bölgeye yaslanır (`state.anchorEnd`), geri kalanı sürükleyerek gezilir. Kaydırma `state.unitsPerPx` ile piksel → sahne birimi çevrilir ve sahne sınırlarına kırpılır; `#zoom-reset` en güncel bölgeye döner. Tüm birleşmiş dallar gösterilir (eski "son 8 yay" katlaması kaldırıldı); sahne yüksekliği/genişliği sabit üst sınıra takılmaz.
- **`web/app.js`:**
  - Ana kokpit orkestratörü, yaşam döngüsü ve olay dinleyicileri.
  - `apiCall(path, method, body)`: Tüm POST isteklerine `X-Requested-With: gitarc` başlığı ekler.
  - 4 saniyelik sessiz canlı polling (`setInterval`, `snapshotSignature(snap)` değişirse `applySnapshot` çağırır; kullanıcı `INPUT`/`TEXTAREA`/`SELECT` odaklıyken veya modal/drag açıkken DOM'u ezmez).
  - `setupBottomDock()` & `renderRadar()`: Alt çekmece sekmeleri ve tam genişlikte, etkilenen dosya etiketli, CI kırılma özetli ve tek tık aksiyonlu hata radarı kartları render edilir.
  - `state.pendingAutoMerge`: `CI: RUNNING` iken Merge tetiklendiğinde dalı kuyruğa alır; polling `CI: SUCCESS` gördüğünde `executeMergeNow` çağırır, `FAILURE` veya çakışma görürse iptal eder.
  - **Yerel Kalıntı & Hata Geçirmezlik Katmanı:** `renderSVG()` yerelde kopyası duran birleşmiş dallar için alt raya kesikli `.svg-local-remnant-path` + `.cap-local-remnant` rozeti çizer; `suggestUniqueBranchName(arg1, arg2, arg3)` değişen dosyalardan dinamik (`feat/<dosya>-guncel`), geçmiş veya açık dallarla çakışmayan benzersiz dal adı üretir.
  - **Konsolsuz Dal Açma & Commit:** Kirli çalışma alanını yerelde `git checkout -b <dal>` akışıyla hatasız yeni dala alıp tek tıkla commit & PR açar.

---

## 4. Yeni Özellik Ekleme Protokolü (Adım Adım Tarif)

Ekonomik modeller bir katmanı atlayıp tutarsızlık üretmemesi için her yeni özellikte şu 5 adımı sırayla izlemelidir:

1. **Veri Modeli (`internal/model/model.go`):** Gerekli JSON alanlarını `camelCase` etiketiyle (`omitempty` uygunsa ekleyerek) tanımla.
2. **Motor (`internal/gitengine/*.go`):**
   - `Manager` üzerinde metodu ekle.
   - Hem **canlı Git/GitHub (`*Live`)** akışını hem **simülasyon (`simRepo`)** akışını yaz.
   - `-team` modunda paylaşılan çalışma dizini `HEAD` değiştirecek bir işlemse en başta `m.IsTeamMode()` kontrolü koy.
   - İşlem sonucunu `m.recordLog` / `m.logLiveOutcome` ile kaydet.
3. **HTTP API (`main.go`):** `mux.HandleFunc("/api/...", a.guard(a.handle...))` ekle ve `decodePOST` ile JSON gövdesini doğrula (`MaxBytesReader` 1 MB sınırı uygulanır).
4. **Frontend (`web/tasarim.css`, `web/index.html`, `web/app.js`):**
   - Yeni görünümler için stilleri **`web/tasarim.css`** içine yaz; **HTML veya JS içinde asla `style=` kullanma**.
   - `web/app.js` içinde `LocalFallbackEngine` karşılığını da ekle.
   - Modelde yeni değişebilen bir alan varsa **`snapshotSignature(snap)`** içine mutlaka dahil et (yoksa 4 saniyelik canlı güncelleme UI'ı yenilemez!).
   - Tüm kullanıcı verilerini DOM'a basarken **`esc(...)`** ile HTML escape et.
5. **Doğrulama & Testler:**
   - İlgili `*_test.go` dosyasına test ekle (`newTempRepo(t)` / `t.TempDir()` ile gerçek geçici git reposu kur). `gh` gerektiren akışlar için `newGHStub(t)`, SVG yerleşim değişiklikleri için `tests/js/canvas.test.mjs` kullan (Bölüm 8).
   - `go vet ./... && go test -count=1 -race -cover ./... && go build -o gitarc .` ve `node --check web/*.js web/modules/*.js` çalıştır.

---

## 5. Hata Geçirmezlik Prensibi (Foolproof UX & Savunmacı Mühendislik Standartları)

GitArc, son kullanıcının Git iç işleyişini veya sırasını ezberlemek zorunda kalmayacağı şekilde **"yanlış tıklamayı önleyen ve ön koşulları kendi tamamlayan"** bir arayüz prensibiyle geliştirilir:

1. **Ön Koşulları Kullanıcıya Yükleme, Güvenli Şekilde Otomatik Çöz:**
   - Kullanıcı üzerinde aktif olduğu (`HEAD`) bir dalı silmek veya temizlemek istediğinde (`deleteBranchLive` / `pruneGoneLive`), çalışma ağacı temizse (`DirtyCount == 0` ve `!IsTeamMode()`) *"önce main'e geçin"* hatası vermek yerine arka planda `git switch <defaultBranch>` yapıp silmeyi tek harekette tamamla.
   - Yeni açılan bir dalda değişiklikler henüz commitlenmeden `Push & PR Aç` basıldıysa (`promoteBranchLive`, `AheadMain == 0 && DirtyCount > 0`), otomatik `git add -A && git commit` atıp push + PR aç.
2. **Tekrarlayan veya Geçmişle Çakışan Varsayılan İsim Üretme:**
   - Sabit `'feat/local-changes'` gibi isimler kullanmak, az önce birleşen/silinen dalların üzerine yanlışlıkla commit atılmasına yol açar. UI formunda varsayılan ad üreten her yardımcı (`suggestUniqueBranchName`), değişen dosyalardan anlamlı bir dal adı türetmeli ve `snap.branches` (açık/birleşmiş tüm dallar) içinde aynı isim varsa kısa SHA / sayaç ekleyerek benzersizleştirmelidir.
3. **Uzak Sunucuya Boş veya Bozuk Kod Gönderme (Fail-Fast Kapıları):**
   - `main` ile farkı olmayan (`AheadMain == 0` ve kirli dosyası da olmayan) boş bir dalı GitHub'a pushlayıp `GraphQL: No commits between ...` hatası üretme; push öncesi yerelde durdur ve net Türkçe uyarı ver.
   - Çakışma çözümünü (`ResolveBranchConflict`) izole `git worktree` içinde yaptıktan sonra `go.mod` varsa mutlaka `go vet ./...` doğrulamasından geçir; hata varsa asla commit/push yapma.
4. **Görsel Durum ile Gerçek Git Durumu Birebir Örtüşmeli (Görsel Dürüstlük):**
   - İşlemi kalmamış temiz yeşil parabollerde `🧹` gibi yanıltıcı aksiyon ikonlarını kaldır.
   - Bilgisayarda hâlâ yer tutan birleşmiş dalları ise hem üst sarı kapsülde (`💻 Local` / `HEAD·Local`) hem alt raydaki kesikli kuyrukta (`.cap-local-remnant`) görünür kıl; hem sunucu simülasyonunda hem `LocalFallbackEngine`'de silme sonrası parabolü kaybetmeden yeşile çevir.

---

## 6. Bilinen Tuzaklar (Pitfalls) ve Dikkat Edilecekler

1. **CSP Kırılması (`TestWebAssetsStrictCSPCompliance`):**
   - JS şablon dizgelerinde `style="..."` veya DOM düğümünde `el.style.display = 'none'` gibi kullanımlar (`style="` / `style='`) CSP ihlalidir ve `main_test.go` testini kırar. Bunun yerine `el.hidden = true` veya CSS sınıfı kullan.
2. **`snapshotSignature` Eksikliği:**
   - 4 saniyelik polling, yeni veriyi sunucudan çekse bile `snapshotSignature(nextSnap) === state.lastSig` çıkarsa ekranı yeniden çizmez. UI'da görüntülenen yeni alanları `snapshotSignature` fonksiyonuna ekle.
3. **Çift Başlatılan Eski Binary Sorunu:**
   - `web/` dosyaları `go build` anında ikiliye gömülür (`//go:embed web`). Kod düzenlendikten sonra kullanıcı arka planda eski `./gitarc` sürecini açık bıraktıysa değişiklikleri göremez; mutlaka `go build -o gitarc .` alıp süreç yeniden başlatılmalıdır.
4. **Boş Fark / `No commits between main and ...` Hatası:**
   - Üzerinde hiç yeni commit olmayan (`aheadMain == 0`) bir dal pushlansa bile `gh pr create` başarısız olur (`GraphQL: No commits between ...`). Canlı motor bunu anlamlı hata mesajına dönüştürür; bu korumaları zayıflatma.
5. **Kaynak Kodda `union` Çakışma Çözümü Riski:**
   - `CHANGELOG.md` veya lock dosyalarında `union` (ikisini alt alta koru) güvenli ve önerilir (`★ ÖNERİLEN`), fakat `.go` / kaynak kod dosyalarında iki tarafın aynı satırdaki değişken tanımını (`:=`) alt alta yapıştırarak `no new variables on left side of :=` derleme hatası üretebilir. Bu yüzden `conflict_studio.go` kaynak kod dosyalarında `ours` önerir ve her çözümde `go vet ./...` kapısından geçirir.
6. **Sürüm Numarası Eşliği:**
   - Yeni sürüm kesildiğinde **`VERSION`**, **`internal/gitengine/collector.go` (`AppVersion`)**, **`web/index.html`** (`#btn-update-check` ve `#sec-update-status` metinleri), **`web/modules/topbar.js`** ve **`web/fallback.js`** içindeki `appVersion || '...'` yedekleri ve **`CHANGELOG.md`** başlığı birlikte güncellenmelidir. `TestVersionConsistency` (`version_test.go`) bunların tutarlılığını denetler; biri unutulursa `go test` kırılır. `TestMegaRepoCacheTeamModeAndProtection` içindeki `CheckForUpdates` testi `AppVersion` eşitliğini doğrular.
7. **Önbellek ve sürüm sorgusu:**
   - `web/index.html` içine `tasarim.css?v=0.8.0` gibi elle yazılı sürüm sorgusu **ekleme**: sunucu zaten her statik dosyaya `no-store` verir, ES modülleri (`modules/*.js`) sorguyu hiç almaz ve sürüm yükselince unutulur. `TestStaticAssetsAreNeverCachedAndHaveNoHardcodedVersionQuery` bunu denetler.
8. **Testler gerçek `~/.gitarc` dizinine yazmamalı:**
   - `NewManager`, `recordLog` ve `AddProject` `$HOME/.gitarc/{logs,projects}.json` dosyalarına yazar. Her test paketinde (`gitarc`, `internal/gitengine`, `internal/mcp`) `TestMain` `HOME`/`USERPROFILE`'ı geçici dizine alır. Yeni bir test paketi eklersen aynı `TestMain`'i ekle, yoksa testler geliştiricinin gerçek loglarını ve proje listesini kirletir/siler.
9. **Şerit sınırı yok:**
   - Yeni kod `MaxHemisphereLanes`/`lane <= 4` gibi bir üst sınır varsaymamalı (`yOfLane`, `computeBranchGeometry`, `AssignLanes`, `fallback.js`). Mega-repo demosunda sahne dikeyde uzar; bu bilinçli bir tercihtir (`TestAssignLanesNoLaneCap`, canvas testleri korur).
10. **AI öneri onayı (`ApproveProposal`) kuralları:**
    - `POST /api/proposal/approve` → `Manager.ApproveProposal(proposalId, branchName, message, prTitle, prBody)`. Kullanıcı yalnızca **metinleri** düzenler; dosya listesi HER ZAMAN önerinin kendisinden gelir (istemci `files` alanı yok sayılır). Öneri kimliği eşleşmezse reddedilir, aynı anda tek onay çalışır (`proposalBusy`), Team modunda HEAD değiştirilemez, gizli dosya/birleşmiş dal engelleri `CommitWorkingTree` içinde yeniden uygulanır; hata olursa öneri bekleyen durumda kalır (yeniden denenebilir). Üçlü eşlik: canlı (`commitWorkingTreeLive`), simülasyon (sanal dal, `ChangedFiles` = önerinin dosyaları) ve `fallback.js` `approveProposal` aynı kuralları uygular. Testler: `proposal_approve_test.go` (gerçek git + sahte `gh`), `main_proposal_test.go`, `tests/js/fallback-proposal.test.mjs`.
    - Stüdyoda çalışma alanı tespiti `isWorkingTreeBranch(b)` (utils.js) tek tanımıdır (`local-wip-*` veya `wip/working-tree`); hiçbir yerde üretilmeyen `b.isWorkingTreeWip` alanına bakma. `tests/js/inspector-cards.test.mjs` her stüdyo butonu kimliğinin `STUDIO_ACTION_IDS` içinde olduğunu denetler (ölü düğme bırakmaz).
11. **Yavaş ağ işlerini snapshot içinde bloklama (arka plan + bayat veri):**
    - `git fetch --prune` ve tüm `gh` sorguları `collector_github.go` içindeki `repoBG` durumunda **arka planda, tek uçuşla** çalışır (`fetchRemoteCached`, `fetchGHMetadataCached`). `CollectLiveRepo` yalnızca `fetchWait` (1 sn) / `ghWait` (2 sn) bekler; sonuç gelmediyse eldeki son başarılı veriyi döndürür. Yeni bir ağ sorgusu eklersen aynı kalıbı kullan ve `SyncStep` kaydet.
    - **Başarısız sorgu eldeki veriyi ezmez** ve boş sonuç önbelleğe yazılmaz (eski kod bunu yapıyordu). `InvalidateGHCache` veriyi silmez, süresi dolmuş işaretler.
    - Sonuçlar `repoBG.version` ile `QuickRepoSignature`'a girer; arayüz kendiliğinden yenilenir. `Repo.SyncSteps` (`fetch`, `prs`, `merged`, `protection`) ve `Repo.ProtectionState` (`active`/`none`/`unknown`) snapshot'ta taşınır; `snapshotSignature` bunları içerir. Prometheus'ta `gh pr list --json statusCheckRollup` ~5 sn sürer (eski 4 sn sınırı hep aşılıyordu).
    - GitHub 404 (yönetici değilsin) **"Korumasız" değil "unknown"**dır. Simülasyon ve `fallback.js` aynı `syncSteps`/`protectionState` şeklini üretir. Testler: `bgsync_test.go` (yavaş `gh`, başarısız yenileme, bayat veri, tek uçuş, 404, sim eşliği); stub `slow(key, saniye)` ile yavaşlatılır.
12. **`state.js` kimlikleri `index.html`'de olmalı:**
    - Eşleşmeyen bir kimlik sessizce `undefined` olur ve `if (el.x)` bloğu hiç çalışmaz (senkron rozeti bu yüzden hiç güncellenmedi). `tests/js/registry.test.mjs` bunu istisnasız denetler: kayıt defterindeki her kimlik `index.html`'de bulunmalı ve kodda okunan her `el.<ad>` `state.js`'te tanımlı olmalı. Bir öğeyi `index.html`'den kaldırırsan kayıt defteri girdisini ve ona bağlı kodu da sil.
13. **Merge noktası bilinmeyen birleşmiş dal çizme (`HistoryOnly`):**
    - Omurga yalnızca son 12 first-parent commit'tir (`collector.go`, `-n 12`). Merge commit'i bu pencerede olmayan birleşmiş dallar (squash vb.) için `EndSpineIndex: lastSpine` ile sahte bir konum atanır; bu dallar `HistoryOnly: true` ile işaretlenir ve **asla parabol olarak çizilmez** (aksi halde Prometheus gibi depolarda 40 dal tek bir sahte dağ oluşturur). Yeni bir "birleşmiş dal" üretim yolu eklersen merge konumunu gerçekten bulamıyorsan `HistoryOnly` set et. `TestMergedBranchOutsideSpineWindowIsHistoryOnly` gerçek repoyla bunu denetler. Çevrimdışı `LocalFallbackEngine` demosunda hep gerçek merge noktası olduğundan bu alanı üretmez; çip arayüzü ortaktır.
14. **Açık tema ve koyu bloklar:**
    - Alt çekmece/stüdyo sütunlarının zeminleri `--terminal-bg` / `--diff-bg` değişkenleridir (koyu: `#070a11`/`#090d16`, açık: `#eef1f6`/`#f8f9fc`); sabit renk kodu yazma, yoksa açık temada siyah şerit çıkar. Log/CI kutuları (`--log-bg`, `.terminal-box`) açık temada bilinçli olarak koyu kalır (IDE terminali görünümü).
15. **`go test` kapsamı ve node:**
    - `canvas_js_test.go` `node --test tests/js/` komutunu çalıştırır; node yoksa test atlanır (CI `ubuntu-latest` üzerinde node hazırdır). `tests/js/` `web/` dışındadır, bu yüzden ikiliye gömülmez ve CSP testine takılmaz.

---

## 7. Sıradaki Geliştirme Fikirleri (Backlog)

1. Kullanıcının sonradan ileteceği ek **UI geliştirme fikirleri** (kullanıcı tarif ettiğinde ele alınacak).
2. Ekran görüntüleri (`docs/` altında) eklenerek zenginleştirilmiş **`README.md` güzelleştirmesi**.
3. **Testsiz kalan yollar:** `gh` kurulu olmayan ortam dalları (`PATH`'ten `gh` çıkarılarak denenmeli) ve Windows/Linux dışı işletim sistemi dalları (`handleChooseFolder` yalnızca çalışılan OS dalı için kapsanır).
4. **`CHANGELOG.md`:** şerit sınırının kaldırılması, dinamik omurga yerleşimi ve test altyapısı (`0.8.0` altında veya yeni sürümde) henüz kayda geçmedi.

---

## 8. Test Altyapısı

| Katman | Konum | Ne test eder |
| :--- | :--- | :--- |
| Go birim/entegrasyon | `internal/gitengine/*_test.go` | Gerçek geçici git depolarıyla mutasyonlar (`newTempRepo`), `lanes_test.go` (şerit paketleme), `gaps_test.go` (proje yönetimi, sync, merge, sürüm kontrolü), `helpers_test.go` (saf yardımcılar + canlı create/checkout/amend) |
| Sahte `gh` stub'ı | `internal/gitengine/ghstub_test.go`, `gh_stub_test.go` | PATH başına geçici bir `gh` kabuk betiği konur; çıktı/çıkış kodu `stub.set("pr_list_open", json, 0)` ile ayarlanır, çağrılar `stub.called(...)` ile doğrulanır. GitHub uzak deposu, yolunda `github.com` geçen **yerel bare depodur** (`newGitHubLikeRepo`), yani hiçbir push/fetch diskin dışına çıkmaz. Windows'ta atlanır |
| HTTP/güvenlik/süreç | `main_test.go`, `main_guard_test.go`, `main_handlers_test.go`, `main_os_test.go` | Host/Origin/CSRF/çerez, `decodePOST` (405/400/1 MB), simülasyon ve canlı depoda uçtan uca handler akışı, statik dosya başlıkları, CSP, `main()` uçtan uca (test ikilisi `GITARC_TEST_RUN_MAIN=1` ile kendini yeniden çalıştırır: port yedeği, `-team`, `-demo`, `-mcp`, SIGTERM), `openBrowser` ve `handleChooseFolder` (sahte `open`/`zenity`/`osascript`) |
| MCP | `internal/mcp/server_test.go` | JSON-RPC araçları |
| Frontend (JS) | `tests/js/*.test.mjs` (`layout`, `canvas`, `filters`, `capsule`, `kpi/topbar`, `scene`, `interactions`, `inspector-model`, `inspector-cards` (kartlar + stüdyo), `syncsteps`, `registry`, `theme`, `modals`, `rails`) (+ `canvas_js_test.go` sarmalayıcısı) | Saf modeller ve kurallar (yerleşim, süzgeç, kapsül kararları, kaydırma/yakınlaştırma, sayaçlar, tema, kayıt defteri); Node yerleşik `node:test`, npm bağımlılığı yok. `web/modules/canvas.js` artık yoktur |
| İzolasyon | her pakette `main_test.go` / `main_home_test.go` | `TestMain` → `HOME` geçici dizin |

**Yeni `gh` davranışı test ederken:** `newGHStub(t)` çağır, anahtarlar: `pr_list_open`, `pr_list_merged`, `pr_list_merged_head`, `pr_create`, `pr_merge`, `pr_edit`, `pr_close`, `run_view`, `api_protection_get`, `api_protection_put`, `api_rulesets`. Her mutasyon testinde `InvalidateGHCache(dir)` çağırmayı unutma (10 sn önbellek).
