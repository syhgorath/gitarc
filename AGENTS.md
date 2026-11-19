# GitArc Çalışma Alanı Kuralları ve Mühendislik Standartları

> Yeni veya daha ekonomik modeller bu projede çalışırken önce kök dizindeki `HANDOVER.md` dosyasını okumalıdır.

## 1. Commit ve Pull Request İş Akışı (Kalıcı Kullanıcı Tercihi)

Kullanıcı bu projede her geliştirme, hata düzeltme veya özellik ekleme oturumunun sonunda Git ve GitHub işlemlerini kendisi tek tıkla (kopyala-yapıştır ile) çalıştırmak istemektedir:

- Değişiklikleri tamamladıktan ve doğrulama komutunu (`go vet ./... && go test -count=1 -v ./... && go build -o gitarc .`) çalıştırdıktan sonra (tam komut seti için Bölüm 4) **otomatik commit veya push yapma** (kullanıcı açıkça istemedikçe).
- Bunun yerine her zaman yanıtın sonunda aşağıdaki kalıpta hazır, tek tıkla kopyalanabilir komut blokları ve açıklamalar sun:
  1. **Dal / Commit Komutu:** Önerilen branch adı (`git checkout -b ...`), `git add`, anlamlı Conventional Commit başlıklı `git commit -m "..."` ve `git push -u origin ...` komutu.
  2. **PR Başlığı (Tek Satır):** GitHub Web arayüzüne yapıştırılacak yalın PR başlığı.
  3. **PR Açıklaması / Gövde Metni (Markdown):** Ne, neden değişti ve doğrulama adımlarını içeren, GitHub Web arayüzüne yapıştırılacak hazır Markdown gövdesi (`gh pr create` yerine web tercih edilmektedir).

## 2. Kırmızı Çizgiler (Asla İhlal Etme)

1. **Sıfır Dış Bağımlılık (`go.mod`):** Projede yalnızca Go standart kütüphanesi (`stdlib`) ve `//go:embed web` kullanılır. Harici Go paketi, `npm`/`node_modules` veya CDN linki ekleme. (Node'un **yerleşik** `node --test` koşucusu ve `node:assert` serbesttir; `tests/js/` altındaki testler yalnızca bunları kullanır ve `package.json` gerektirmez.)
2. **Sıkı CSP (`style-src-attr 'none'`):**
   - `web/index.html`, `web/app.js`, `web/fallback.js` ve `web/modules/*.js` içinde **ASLA** satır içi `style="..."` veya `style='...'` özniteliği yazma (`el.style.*` ataması da yapma).
   - `web/*.html` içinde harici `http://` veya `https://` URL kullanma (`http://www.w3.org/2000/svg` SVG XML namespace hariç).
   - Tüm görsel durumları `web/tasarim.css` içinde CSS sınıfı olarak tanımla (`hidden`, `.running-pulse`, `.ci-running`, `.ci-fail` vb.). `main_test.go` içindeki `TestWebAssetsStrictCSPCompliance` bunu her `go test` koşusunda zorunlu denetler.
3. **Üçlü Motor Eşliği (Tri-Mode Parity):** Eklenen her yeni özellik veya aksiyon şu 3 yerde birden çalışmalıdır:
   - **Canlı Git/GitHub Motoru:** `internal/gitengine/collector.go` + `internal/gitengine/simulation.go` (`*Live` metotlar)
   - **Sunucu Simülasyon Motoru:** `internal/gitengine/simulation.go` (`simRepo` bellek-içi durumu)
   - **Tarayıcı Offline Yedek Motoru:** `web/fallback.js` içindeki `LocalFallbackEngine` (yerleşim mantığı gibi paylaşılan davranışlar `internal/gitengine/lanes.go` ile birebir aynı kalmalıdır; şerit sayısı **sınırsızdır**, üst sınır ekleme)
4. **Zero-Touch Çalışma Dizini Kuralı:** Çakışma algılama (`git merge-tree --write-tree`) ve çakışma çözümü (`ResolveBranchConflict` geçici izole `git worktree`) kullanıcının aktif çalışma ağacındaki (`HEAD`) kirli dosyalara asla dokunmamalıdır. Çözüm pushlanmadan önce `go.mod` varsa `go vet ./...` doğrulamasından geçirilmelidir.
5. **Team Server Modu (`-team`):** `m.IsTeamMode()` aktifken paylaşılan `HEAD`'i değiştiren işlemler (`CheckoutBranch`, `AmendLastCommit`, `CommitWorkingTree`) engellenir; izole `worktree` çözümü, uzak dal silme/merge işlemleri çalışmaya devam eder.
6. **Güvenlik Kalkanları (`main.go`):** `Host` (DNS Rebinding), `gitarc_session` (`HttpOnly`, `SameSite=Strict`), `POST` isteklerinde `X-Requested-With: gitarc` + `Origin` kontrolü ve `validateBranchName`/`sanitizeID` girdi doğrulamaları korunmalıdır.
7. **Binary Yeniden Derleme:** `web/` dosyaları `//go:embed web` ile ikiliye gömülür; her UI veya Go değişikliği sonrası `go build -o gitarc .` çalıştırılmalıdır.
8. **Test Hijyeni:**
   - Her test paketinde `TestMain` `HOME`/`USERPROFILE`'ı geçici dizine almalıdır; aksi halde `NewManager`/`recordLog`/`AddProject` geliştiricinin gerçek `~/.gitarc/logs.json` ve `projects.json` dosyalarına yazar. Yeni test paketi eklersen aynı `TestMain`'i ekle.
   - Testler gerçek `gh`'yi, ağı veya gerçek GitHub'ı kullanmaz: `gh` akışları için `newGHStub(t)` (`internal/gitengine/ghstub_test.go`), GitHub uzak deposu için yolunda `github.com` geçen **yerel bare depo** (`newGitHubLikeRepo`) kullan.
   - `web/index.html` içine elle yazılı `?v=...` sürüm sorgusu ekleme; sunucu zaten `no-store` verir (`TestStaticAssetsAreNeverCachedAndHaveNoHardcodedVersionQuery`).
9. **Hata Geçirmezlik Prensibi (Foolproof UX & Otomatik Ön Koşul Çözümü):**
   - **Kullanıcıyı Çıkmaz Sokağa Sokma:** Bir aksiyon öncesinde Git ön koşulu gerekiyorsa (örn. silinecek dalın aktif `HEAD` olması, PR açılacak dalda kaydedilmemiş kirli dosyaların bulunması) çalışma ağacı temiz/güvenli olduğu sürece kullanıcıya *"önce başka dala geçin"* veya *"önce commit atın"* hatası döndürme; arka planda güvenli ön adımı (`git switch main`, `git add -A && git commit`) otomatik uygula.
   - **Taze Yerel Dal Koruması:** `aheadMain == 0` olan taze yerel dallar asla `origin: gone` / silinmiş kalıntı sayılmaz; `🌱 Yeni Yerel Dal` olarak alt rayda açık mor parabolle gösterilir, eylemlerinde silme yerine `🎯 Dala Geç` ve `🚀 Push & PR Aç` yer alır.
   - **Mükerrer Dal Açma Engeli:** Hem arayüzde hem API'de mevcut veya birleşmiş dal adlarıyla dal açılması engellenir.
   - **Pürüzsüz `checkout -b` Stüdyo Akışı:** Çalışma ağacında kirli dosya varken dal açma stüdyosu `git switch -c ... origin/main` yerine güvenli yerel `git checkout -b <dal>` akışıyla çalışır.
   - **Benzersiz Varsayılan Öneriler:** Kullanıcıya dal veya etiket adı önerirken açık ve birleşmiş (`merged`/`gone`) tüm dalları denetle (`suggestUniqueBranchName`); statik isimler yerine değişen dosyalardan dinamik (`feat/<dosya>-guncel`) isimler türet.
   - **Uzak Sunucuya Boş/Bozuk İstek Gönderme:** `gh pr create` veya `git push` öncesi `AheadMain == 0` ya da `go vet ./...` doğrulamasını yerelde çalıştır; ham GraphQL/CLI hatası sızdırmak yerine ne yapılması gerektiğini açıklayan net Türkçe uyarı döndür.
   - **Yavaş Ağ İşini Bloklama / Sessiz Boş Sonuç:** `git fetch` ve `gh` gibi yavaş olabilen sorguları anlık snapshot içinde sıkı zaman aşımıyla çalıştırıp hatayı yutma ve boş sonucu önbelleğe yazma. Arka planda (tek uçuş) çalıştır, son başarılı veriyi koru, durumu `SyncSteps` ile görünür kıl. "Okunamadı" ile "yok/korumasız" farklı şeylerdir (`protectionState: unknown`).
   - **Uydurma Konum Çizme:** Gerçek merge/konum bilgisi bulunamayan birleşmiş dalı sahte bir noktaya sabitleyip parabol çizme; `HistoryOnly` ile işaretle, tuvalin sol üstündeki `🗂 Geçmiş` çipinde listele (`TestMergedBranchOutsideSpineWindowIsHistoryOnly`). Birleşmiş yayların uç genişletmesi `MAX_MERGE_SPREAD` ile sınırlı kalmalıdır.
   - **Görsel Dürüstlük:** Eylemi olmayan öğede gereksiz buton gösterme; yalnızca yerel kalıntısı duran birleşmiş parabollerde `💻 Local` / `HEAD·Local` rozeti, alt ray kalıntı kuyruğu ve `🧹` butonu göster, tamamen temiz yeşil kapsüllerde `🧹` butonunu gizle.

## 3. Mimari ve Dosya Haritası (`v0.9.0`)

- `main.go` / `main_test.go`: HTTP sunucusu, güvenlik middleware (`root`, `guard`), 34 `/api/*` rota kaydı (takma adlar dahil), `-addr`, `-repo`, `-open`, `-mcp`, `-team` bayrakları. Testler: `main_test.go` (CSP & güvenlik akışı), `main_guard_test.go` (oturum, `decodePOST`, Origin), `main_handlers_test.go` (canlı/sim handler akışı, statik başlıklar), `canvas_js_test.go` (JS testlerini `go test` içinden çalıştırır).
- `internal/model/model.go`: `CockpitSnapshot`, `RepoMeta`, `CommitNode`, `BranchArc`, `PRStatus`, `CIStatus`, `ConflictDetail`, `ConflictStudioPayload`, `ActionLogEntry`.
- `internal/gitengine/collector.go`: `AppVersion` (`0.9.0`), `CollectLiveRepo`, SHA-bazlı `evalCache`, 40-dal sıralı penceresi, `collectLocalBranches` (taze yerel dal ayrımı), `collectRemoteOnlyBranches`, `ParseMergeTreeOutput`, `BuildSuggestedCommand`.
- `internal/gitengine/simulation.go`: `Manager`, in-memory `simRepo` sanal durum yönetimi, `SyncSpine`, `ProtectDefaultBranch`, `CheckForUpdates`, kalıcı `actionLogs`.
- `internal/gitengine/live_mutations.go`: Canlı Git/GitHub mutasyonları (`commitWorkingTreeLive` yerel `checkout -b` akışı, `promoteBranchLive`, `mergeBranchLive`, `deleteBranchLive` & `pruneGoneLive` otomatik `HEAD → main` geçişi, `updatePRMetaLive`, `amendLastCommitLive`).
- `internal/gitengine/collector_github.go`: `gh` ile açık/birleşmiş PR, CI ve Branch Protection/Ruleset sorgusu (10 sn `ghCache`; her mutasyondan sonra `InvalidateGHCache`).
- `internal/gitengine/prunable.go`: `IsPrunable` — 🧹 Temizle sayacı ve temizliği için tek kural (`BranchArc.prunable`; sayaç `b.prunable` sayar, kendi koşulunu yazma; `fallback.js` aynı kuralı taşır).
- `internal/gitengine/basebranch.go`: `SetBaseBranch` — omurga dalının kalıcı elle seçimi (`/api/repo/base-branch`); `detectDefaultBranch` önce seçimi, sonra `origin/HEAD`'i kullanır. Simülasyon/fallback reddeder.
- `internal/gitengine/longlived.go`: `IsLongLivedBranch` — develop/release/* gibi dallar `PruneGone`/`DeleteBranch`/kalıntı tanılamasından korunur (`fallback.js` ile aynı kural, üç motorda eşlik).
- `internal/gitengine/conflict_studio.go`, `diagnostics.go`, `lanes.go`: İzole 3-yollu `git merge-file` stüdyosu ve akıllı öneri, ters indeks Cross-PR çarpışma ve CI log ayıklama, deterministik interval packing şerit yerleşimi (sınırsız şerit).
- `internal/gitengine/*_test.go` (73 üst düzey test, kapsam %82): `engine_test.go` (gerçek geçici bare/clone depolarıyla mutasyon ve iş akışları), `lanes_test.go`, `gaps_test.go` (proje yönetimi, sync, merge, sürüm kontrolü), `helpers_test.go` (saf yardımcılar, `newTempRepo`), `ghstub_test.go` + `gh_stub_test.go` (sahte `gh` stub'ı), `main_test.go` (`HOME` izolasyonu).
- `internal/mcp/server.go` & `server_test.go`: Sıfır bağımlılıklı `stdio` JSON-RPC 2.0 MCP sunucusu (`gitarc_status`, `gitarc_check_conflicts`, `gitarc_ci_failure_digest`, `gitarc_sync_main`, `gitarc_resolve_conflict`).
- `tests/js/*.test.mjs`: yerleşim, süzgeç, kapsül, sahne, etkileşim, senkron adımları, kayıt defteri ve tema kuralları için Node yerleşik testleri (168 test; `web/` dışında olduğundan ikiliye gömülmez).
- `web/index.html`, `web/tasarim.css`, `web/fallback.js`, `web/app.js` (orkestratör) ve `web/modules/*.js` (`state`, `constants`, `utils`, `api`, `layout`, `filters`, `capsule`, `kpi`, `topbar`, `syncsteps`, `polling`, `theme`, `scene`, `interactions`, `rails`, `inspector`, `inspector-model`, `inspector-cards`, `studio`, `proposal`, `inspector`, `dock`, `conflict-studio`, `project-modals`, `tour`, `mcp-modal`, `diff-viewer`): Parabolik SVG kokpit, **PocketUI Alt Çekmece (Bottom Dock)** (`#bottom-dock` · Radar / Loglar / Son İşlemler), **Tam Genişlik Hata Radarı Kartları** (`.radar-card`), alt ray yerel kalıntı kuyruğu (`.svg-local-remnant-path` / `.cap-local-remnant`), Sağ Inspector (`suggestUniqueBranchName`), `CI: RUNNING` nabız + `pendingAutoMerge` kuyruğu, modal stüdyolar ve modüler `LocalFallbackEngine` (`fallback.js`).

## 4. Zorunlu Doğrulama Adımları (Her Görev Sonu)

Değişiklik yaptıktan sonra yanıt vermeden önce mutlaka şu komutları çalıştırıp tüm testlerin sıfır hatayla geçtiğini gör (CI `-race` ile çalışır; `go test` node kuruluysa `tests/js` testlerini de çalıştırır):
```bash
go vet ./... && go test -count=1 -race -cover ./... && go build -o gitarc .
for f in web/*.js web/modules/*.js; do node --check "$f"; done
```
Ayrıca yeni eklediğin her Go iş mantığı veya bugfix için ilgili `*_test.go` dosyasında (`engine_test.go`, `gaps_test.go`, `lanes_test.go`, `main_*_test.go`, `server_test.go`) otomatik test yaz; `gh` gerektiren akışlar için `newGHStub(t)`, SVG yerleşim/geometri değişiklikleri için `tests/js/canvas.test.mjs` kullan. Ayrıntılar `HANDOVER.md` Bölüm 8'dedir.
