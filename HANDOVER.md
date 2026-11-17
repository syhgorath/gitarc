# GitArc Teknik Devir Teslim ve Mühendislik Rehberi (`HANDOVER.md`)

> **Mevcut Sürüm:** `0.8.0` (`VERSION` ve `internal/gitengine/collector.go` `AppVersion`)  
> **Modül / Go Sürümü:** `module gitarc` · `go 1.27.3` · Sıfır dış bağımlılık (`go.mod`)  
> **Son Doğrulanmış Test Durumu:** `go vet ./... && go test -count=1 -v ./... && go build -o gitarc .` → **20/20 test geçti** (`gitarc`: 2, `gitarc/internal/gitengine`: 17, `gitarc/internal/mcp`: 1, `gitarc/internal/model`: test dosyası yok), `node --check web/app.js` → **0 hata**.

---

## 1. Proje Özeti ve Görsel Parabolik Metafor

GitArc, standart düz `git log` ağaçları yerine Git ve GitHub yaşam döngüsünü **parabolik ray topolojisi** ile gösteren, sıfır bağımlılıklı bir Go + Vanilla SVG/JS yerel masaüstü/ekip kokpitidir:

- **Merkez Yatay Ray (`main` / `defaultBranch`):** Üzerindeki commit düğümleri (`CommitNode`), tag'ler ve `origin/main` ile senkron farkı (`Ahead` / `Behind`).
- **Alt Yarı-Küre (`hemisphere: "lower"`, kesikli mor yay):** Henüz uzak sunucuya (`origin`) pushlanmamış ve PR açılmamış yerel dallar + kirli çalışma ağacını gösteren sanal `local-wip-working-tree` (`wip/working-tree`) yayı.
- **Üst Yarı-Küre (`hemisphere: "upper"`, mavi/kırmızı/turuncu/kehribar yay):** Uzak dala pushlanmış, açık Pull Request ve GitHub Actions CI durumunu taşıyan aktif dallar (yerelde kopyası silinmiş `origin/*` / GitHub-only PR'lar dahil).
- **Tam Kapanmış Parabol (`state: "merged"`, yeşil yay):** `baseSha` noktasından çıkıp `mergeSha` noktasında yeniden `main` omurgasına dönen birleşmiş dallar.
- **Akıllı Hata Radarı & Çakışma Kapsülleri:** Başarısız CI (`ci-fail`), merge çakışması (`conflict`), çalışan CI (`ci-running` nabız efekti), Cross-PR dosya çarpışması ve `origin: gone` kalıntıları üstünde eylem düğmeleri taşır.

---

## 2. Sürüm Kronolojisi (`v0.1.0` → `v0.8.0`)

| Sürüm | Öne Çıkan Özellikler |
| :--- | :--- |
| **v0.1.0 – v0.2.0** | Parabolik SVG motoru, Canlı Git + Simülasyon ikili motoru, `git merge-tree --write-tree` ile çalışma dizinine dokunmayan çakışma ve Cross-PR algılama, gürültüsüz CI log ayıklama, tek tıkla Merge/Squash/Rebase. |
| **v0.3.0** | Gömülü `stdio` JSON-RPC 2.0 MCP sunucusu (`./gitarc -mcp`), açık PR başlık/açıklama düzenleme (`gh pr edit`), `HEAD` son commit `--amend --only` düzenleme, `git switch` checkout ve Terminal komutu kutusu. |
| **v0.4.0** | Uzak (`origin/*` / GitHub-only PR) dal keşfi (`collectRemoteOnlyBranches`), remote-only çakışma algılama, `gh pr close --delete-branch` / `git push origin --delete` temizliği, gerçek bare/clone git deposu mutasyon testleri. |
| **v0.5.0** | **Çakışma Çözüm Stüdyosu** (`/api/conflict/inspect`): Sol (Olan 3-yollu diff hunks) / Sağ (Olacak canlı önizleme), 4 strateji (`union`, `ours`, `theirs`, `custom`), izole geçici `git worktree` içinde çözüm + `go vet ./...` güvenlik kilidi, Antigravity MCP bağlantısı (`gitarc_resolve_conflict`, `.agents/plugins/gitarc/`). |
| **v0.6.0** | Kalıcı `📜 Loglar` modalı (`CockpitSnapshot.Logs` + `/api/logs/clear`), stüdyo içi kalıcı kopyalanabilir hata kutusu (`#studio-error-banner`), dosya türüne göre `★ ÖNERİLEN` strateji danışmanı. |
| **v0.7.0** | **Mega-Repo (10K Geliştirici) Ölçekleme:** SHA-bazlı `evalCache` (`CacheHits`), `--sort=-committerdate --count=40` dal penceresi, $O(N \cdot F)$ ters indeks Cross-PR motoru, `👤 Benim Dallarım (@me)` ve `📂 Klasör/Dosya Merceği`, `-team` ekip sunucusu modu, Windows `rundll32` tarayıcı açılışı, Branch Protection (`/api/repo/protect`) ve Update Checker (`/api/system/update-check`). |
| **v0.8.0** | **PocketUI Alt Çekmece (Bottom Dock)** (Radar/Loglar/İşlemler), **Tam Genişlik Hata & Uyarı Kartları** (dosya etiketleri, CI hata logu snippet'i, kopyalanabilir komut satırı), **Taze Dal Ayrımı** (`🌱 Yeni Yerel Dal` · `aheadMain == 0` dalların yanlışlıkla `origin: gone` sayılmasını engelleme), **Mükerrer Dal Açma Kalkanı**, **Konsolsuz Commit & PR Stüdyosu** (`/api/working-tree/commit`, yerel `git checkout -b` akışı), **Push Öncesi PR Başlık & Açıklama** (`PromoteBranchWithMeta`), **`CI: RUNNING` Nabız Efekti** (`.running-pulse`), polling tabanlı **Otomatik Merge Kuyruğu**, **Alt Ray Yerel Kalıntı Kuyruğu** + `💻 Local` rozeti, otomatik `HEAD → main` geçişli `DeleteBranch`/`PruneGone`. |

---

## 3. Dosya ve Mimari Haritası

### Go Backend (`main.go`, `internal/`)
- **`main.go`:**
  - Bayraklar: `-addr` (`127.0.0.1:7432`), `-repo` (`.`, demo için `"sim"`), `-open` (`true`), `-mcp` (`false`), `-team` (`false`).
  - Statik dosyaları `//go:embed web` ile gömer ve kök yanıtta sıkı CSP başlığı set eder:
    `default-src 'none'; script-src 'self'; style-src 'self'; style-src-attr 'none'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`
  - **23 `/api/*` uç noktası** (`a.guard(...)` arkasında):
    - `GET /api/snapshot`
    - `POST /api/repo/switch`, `/api/repo/add`, `/api/repo/remove`
    - `POST /api/branch/create`, `/api/branch/checkout`, `/api/branch/merge`, `/api/branch/promote`, `/api/working-tree/commit`, `/api/working-tree/discard`, `/api/working-tree/diff-stat`
    - `POST /api/conflict/inspect`, `/api/branch/resolve`, `/api/branch/fix-ci`
    - `POST /api/branch/delete`, `/api/branch/prune`, `/api/pr/edit`, `/api/commit/amend`
    - `POST /api/spine/sync`, `/api/logs/clear`, `/api/system/update-check`, `/api/repo/protect`, `/api/simulation/reset`, `/api/test/run`
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
- **`web/app.js`:**
  - Ana kokpit kontrolcüsü, DOM/SVG render, durum yönetimi ve olay dinleyicileri.
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
   - İlgili `*_test.go` dosyasına test ekle (`t.TempDir()` ile gerçek geçici git reposu kurarak test et).
   - `go vet ./... && go test -count=1 -v ./... && go build -o gitarc .` ve `node --check web/app.js` çalıştır.

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
   - Yeni sürüm kesildiğinde **`VERSION`**, **`internal/gitengine/collector.go` (`AppVersion`)** ve **`CHANGELOG.md`** mutlaka güncellenmelidir; `TestMegaRepoCacheTeamModeAndProtection` içindeki `CheckForUpdates` testi `AppVersion` eşitliğini doğrular.

---

## 7. Sıradaki Geliştirme Fikirleri (Backlog)

Kullanıcıyla konuşulan ama henüz başlanmamış iki başlık:
1. Kullanıcının sonradan ileteceği ek **UI geliştirme fikirleri** (kullanıcı tarif ettiğinde ele alınacak).
2. Ekran görüntüleri (`docs/` altında) eklenerek zenginleştirilmiş **`README.md` güzelleştirmesi**.
