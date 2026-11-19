# Değişiklik Günlüğü (CHANGELOG)

Bu proje [Keep a Changelog](https://keepachangelog.com/tr/1.0.0/) biçimini ve SemVer sürümlemeyi izler.

## [Yayımlanmamış]

### Eklenenler
- **Veri Toplama Adımları (`SyncSteps`) ve 2 Satırlı Senkron Rozeti:** Sağ üstteki `main ↔ origin/main (Synced)` rozetinin altına `git fetch ✓ · PR listesi ✓ 20 · Birleşmiş PR ✓ · Koruma ?` satırı eklendi. Her adım için durum (`ok`, `pending ⏳`, `stale ↻`, `failed ✗`, `unknown ?`, `skipped –`), süre, sayı ve "ne kadar önce" bilgisi tooltip'te görünür. "Synced" yalnızca yerel `origin/main` referansına bakar; bu satır hangi verinin eski veya eksik olduğunu gösterir. Simülasyon ve çevrimdışı motor aynı şekli üretir.
- **Üç Değerli Koruma Durumu (`protectionState`):** `active` / `none` / `unknown`. Yönetici olmayan kullanıcıya GitHub 404 döndürür; bu artık "Korumasız" değil "❔ Koruma ?" (bilinmiyor) olarak gösterilir ve `Korumayı Etkinleştir` önerilmez.
- **DOM Kayıt Defteri Koruma Testi (`registry.test.mjs`):** `state.js` içindeki kimliklerin `index.html`'de var olduğunu denetler; sessiz eşleşme hataları artık testte yakalanır.
- **"Geçmiş" Çipi (`historyOnly`):** Birleşmiş ama merge noktası omurga penceresinde (son 12 commit) bulunamayan dallar (squash ile birleşenler, eski merge'ler) artık parabol olarak çizilmez. Sol raydaki `main` kartının altında `🗂 Geçmiş` çipinde (sayaçlı) listelenir; bir dala tıklamak Inspector'da açar (`🧹 Sil / Temizle` dahil). `Esc` veya dışarı tıklama listeyi kapatır. MegaCorp demosunda 8 örnek geçmiş dal vardır.
- **`BranchArc.historyOnly` alanı:** Canlı motor, merge noktasını bulamadığı birleşmiş yerel ve uzak dalları işaretler; `omitempty` olduğu için eski istemcilerle uyumludur. Bu dallar `AssignLanes` içinde şerit tüketmez.

### Düzeltilenler
- **Büyük depolarda veri geç geliyor / hiç gelmiyordu (Prometheus):** `CollectLiveRepo` 6 sn'lik toplam bütçeyle çalışıyor, içinde `git fetch --prune` (3 sn) ve `gh pr list` (4 sn) vardı. Prometheus'ta `gh pr list --json statusCheckRollup` 4,8 sn sürdüğü için sürekli zaman aşımına uğruyor, hata sessizce yutulup **boş sonuç önbelleğe yazılıyor**, bütçe bitince sonraki git komutları da başarısız oluyordu (PR yok, uzak dal yok, boş radar). Artık `git fetch` ve `gh` sorguları **arka planda** (tek uçuş, bol zaman aşımıyla) çalışır; anlık snapshot yalnızca kısa süre (`fetchWait` 1 sn, `ghWait` 2 sn) bekler ve eldeki son başarılı veriyi gösterir. Başarısız sorgu eldeki veriyi silmez, `InvalidateGHCache` veriyi silmeden "süresi dolmuş" işaretler (bayat gösterim). Sonuç geldiğinde `QuickRepoSignature` değişir ve arayüz kendiliğinden yenilenir.
- **Senkron rozeti hiç güncellenmiyordu:** `state.js` rozeti `spine-sync-badge` kimliğiyle arıyordu; bu bir CSS sınıfı, gerçek kimlik `btn-canvas-sync`. Eşleme boş kaldığı için tüm güncelleme bloğu çalışmıyor, rozet her depoda HTML'deki sabit "(Synced)" metnini gösteriyordu (ahead/behind farkı da yansımıyordu).
- **"Korumasız" yanlış alarmı:** Branch protection sorgusunun 404 (yetki yok) ve zaman aşımı yanıtları "Korumasız (Doğrudan push açık)" olarak gösteriliyordu.
- **Uydurma merge noktası:** Merge commit'i omurga penceresinde olmayan birleşmiş dallar omurganın son düğümüne sabitlenip tam genişlikte parabol olarak çiziliyordu (Prometheus gibi büyük depolarda 40 dal 40 şerit ve tek bir dev "dağ" oluşturuyordu). Artık yalnızca gerçek merge noktası olan dallar çizilir.
- **Yapay uç genişletme:** Birleşmiş yayların iki ucu şerit numarasıyla orantılı olarak (40 şeritte ±1900 px) gerçek merge konumundan uzaklaştırılıyordu. Sapma `MAX_MERGE_SPREAD` (2 omurga adımı) ile sınırlandı.

## [0.9.0] - 2026-11-19

### Eklenenler
- **Koyu / Açık Tema Düğmesi (`theme.js`):** Daha önce işleyicisiz olan `◐` düğmesi bağlandı. Seçim `<html data-theme>` özniteliği ve `localStorage` (`gitarc_theme`) ile saklanır, sayfa yenilenince korunur; depolama kapalıysa koyu temaya düşer. Düğmenin `aria-pressed` ve başlığı güncellenir, satır içi `style` kullanılmaz (CSP uyumlu).
- **Okunabilir Ölçek ve Güncel Bölgeye Yaslanma (`computeView`):** Çok geniş sahneler (örn. MegaCorp enterprise demosu) artık ekrana sığdırılıp okunmaz hale küçültülmez; en az `MIN_READABLE_SCALE` (0,5 px/birim) ölçekte çizilir, ilk açılışta ve depo değişince en güncel (sağ) bölgeye yaslanır, geri kalanı sürüklenerek gezilir. `#zoom-reset` en güncel bölgeye döner.
- **Sürüm Tutarlılık Testi (`version_test.go`):** `VERSION`, `AppVersion`, `index.html`, `canvas.js`, `fallback.js` içindeki elle yazılı sürümler ve CHANGELOG başlığı birbiriyle eşleşmezse `go test` kırılır.
- **Sınırsız Şerit & Dinamik Omurga Yerleşimi (`lanes.go`, `canvas.js`, `fallback.js`):** Çok sayıda çakışan dal artık üst üste yığılmak yerine her biri kendi şeridine yerleşir (eski `MaxHemisphereLanes = 4` sınırı üç motorda da kaldırıldı). Omurga düğümleri dal kalabalığına göre açılır (`layoutSpinePositions`), birleşmiş yayların iki merge noktası kalabalığa ve şeride göre uzaklaşır (`layoutBranchCrowd`), dış yaylar iç yayları kapsayacak biçimde daha yüksek ve geniş çizilir. Tüm birleşmiş dallar gösterilir (eski "son 8 yay" katlaması kaldırıldı); sahne boyutu sabit üst sınıra takılmaz.
- **Diff Görüntüleyici (`/api/diff`, `diff_service.go`):** Commit, dal veya çalışma ağacı için birleşik diff üretimi ve `diff-stat` özeti.
- **AI Commit/PR Teklif Kartı (`proposals.go`, `/api/proposal/dismiss`):** `.git/gitarc_proposal.json` IPC dosyası üzerinden gelen bekleyen teklif kokpitte gösterilir; reddedilince bellekten ve diskten silinir.
- **Güvenlik Kalkanı Yapılandırması (`guard_config.go`):** Secret dosya/içerik desenleri ve `git add -A` yasağı (`StageSafeFiles`) varsayılan olarak devrededir; depo kökündeki `.gitarc` veya `.gitarc.json` dosyasıyla ek desenler tanımlanabilir (`LoadGuardConfig`). Kalıcı loglar `~/.gitarc/logs.json` içinde tutulur.
- **Yeni API uçları:** `GET /api/system/git-status-ping` (ağır git komutu çalıştırmadan değişiklik algılama imzası), `POST /api/system/choose-folder`, ve eski istemciler için `/api/sync-spine`, `/api/reset`, `/api/run-local`, `/api/branch/prune-gone` takma adları.
- **Test Altyapısı:**
  - `main()` uçtan uca (test ikilisi `GITARC_TEST_RUN_MAIN=1` ile kendini yeniden çalıştırır: sim sunucusu, port yedeği, `-team`, `-demo`, `-mcp`, SIGTERM), `openBrowser` ve `handleChooseFolder` (sahte `open`/`xdg-open`/`osascript`/`zenity`) testleri; ana paket kapsamı %37 → %74.
  - `canvas.js` yerleşim ve geometri değişmezleri için sıfır bağımlılıklı Node testleri (`tests/js/`, 36 test: canvas geometrisi ve tema); `go test` içinden de çalışır (`canvas_js_test.go`).
  - Sahte `gh` stub'ı (`ghstub_test.go`): gerçek `gh`, ağ veya GitHub hesabı olmadan `fetchGHPRs`, `fetchGHMergedPRs`, Branch Protection/Ruleset, `promoteBranchLive`, `mergeBranchLive`, `updatePRMetaLive`, `deleteBranchLive` ve `ProtectDefaultBranch` testleri.
  - Şerit paketleme, proje yönetimi, `SyncSpine`, yerel merge/squash, sürüm kontrolü, oturum/CSRF/`decodePOST` ve canlı depoda uçtan uca HTTP handler testleri. `gitengine` kapsamı %62 → %82.

### Değişenler
- **Enterprise Demosu Okunur Hale Geldi:** Omurga genişliği hesaplanırken yalnızca birleşmiş yaylar kalabalığa sayılır; açık dalların kapsülleri zaten ayrı yerleşir (10 açık PR son aralıkları 1000 px'e şişiriyordu). MegaCorp demosunda tarayıcı `viewBox` genişliği 8246 → 2838, omurga son noktası 6716 → 3231; sandbox omurgası 3244 → 2224.
- **README:** ekran görüntüleri (`docs/screenshots/`), parabolik metafor tablosu, bayrak ve MCP araç tabloları, güvenlik modeli ve güncel test komutlarıyla yeniden yazıldı.
- **Frontend modülerleşti:** `web/app.js` (3.470 → ~550 satır) `web/modules/` altındaki ES modüllerine bölündü (`state`, `constants`, `utils`, `api`, `canvas`, `inspector`, `dock`, `modals`); `LocalFallbackEngine` `web/fallback.js` dosyasına taşındı. Go tarafında `simulation.go` büyük dosyası `collector_github.go`, `live_mutations.go`, `projects.go`, `simulation_enterprise_mock.go` ve diğerlerine ayrıldı. Ayrıntı: `REFACTOR_INDEX.md`.
- **Önbellek:** Tüm statik dosyalar `Cache-Control: no-cache, no-store, must-revalidate` (+ `Pragma`, `Expires`) ile sunulur; `index.html` içindeki elle yazılı `?v=` sürüm sorgusu gereksiz olduğu için kaldırıldı.
- **Belgeler:** `HANDOVER.md` ve `AGENTS.md` güncel mimari, test altyapısı ve yeni kurallarla yenilendi.

### Düzeltilenler
- **Sürükleme Birimi:** Tuval sürüklemesi piksel yerine yanlışlıkla `1/zoom` ile çevriliyordu; artık gerçek piksel → sahne birimi dönüşümü (`unitsPerPx`) kullanılır ve kaydırma sahne sınırlarına kırpılır.
- **Açık Temada Siyah Şerit:** Alt çekmece ve stüdyo sütunlarındaki sabit koyu zeminler (`#070a11`, `#090d16`) `--terminal-bg` / `--diff-bg` değişkenlerine taşındı; açık temada siyah şerit ve siyah diff paneli kalmadı (log/CI kutuları bilinçli olarak koyu kalır).
- **Eski Sürüm Metinleri:** `index.html` ve `fallback.js` içinde kalmış `v0.7.0` / `v0.8.0` yedek metinleri güncellendi.
- **Testler gerçek `~/.gitarc` dizinine yazıyordu:** `gitarc`, `internal/gitengine` ve `internal/mcp` test paketlerine `HOME`/`USERPROFILE`'ı geçici dizine alan `TestMain` eklendi; testler artık geliştiricinin gerçek `logs.json` ve `projects.json` dosyalarını değiştirmez.

## [0.8.0] - 2026-11-17

### Eklenenler
- **Etkileşimli Kokpit Turu & Spotlight Rehberi (`🎓 Tur` / `#tour-backdrop`):** Ekranı yarı saydam karartıp adım adım hedef bileşenleri (Depo seçici, Parabolik Raylar, PocketUI Bottom Dock, Çakışma & Commit Stüdyosu, Güvenlik & MCP) aydınlatan 5 adımlı rehber eklendi; klavye ok tuşları (`◀/▶`) ve `Escape` desteği ile sıfır dış kütüphane ve sıkı CSP uyumlu olarak uygulandı.
- **Kalıcı Yerel Proje Yönetimi (`➕ Proje Ekle` / `~/.gitarc/projects.json`):** Kullanıcıların yerel Git depolarını arayüzdeki modal üzerinden yol girerek ekleyebilmesi, eklenen projelerin kalıcı olarak saklanması, üst bardan seçilmesi ve `🗑️` ile listeden çıkarılabilmesi eklendi (`/api/repo/add`, `/api/repo/remove`).
- **Büyük Proje Demosu (MegaCorp Platform · 50 Geliştirici & 4 Ana Dal):** Ekran görüntüsü ve dokümantasyon için 4 ana entegrasyon/sürüm hattı (`develop`, `staging`, `release/v3.3`, `support/lts-2.x`), 50 farklı ekip üyesi, `CI: RUNNING` nabız efekti, panik loglu `CI: FAILURE`, Cross-PR çakışması (`api/v1/auth.go`), Helm values çakışması, squash kalıntısı ve taze yerel dal içeren zengin kurumsal simülasyon deposu (`sim-enterprise`) eklendi.
- **Çalışma Ağacını Güvenle Sıfırlama (`🗑️ Değişiklikleri Geri Al` / `/api/working-tree/discard`):** Staged veya untracked açık değişiklikleri tek tıkla ve onay penceresiyle temizleyen güvenli sıfırlama mekanizması eklendi.
- **PocketUI Tab'lı Alt Çekmece (Bottom Dock):** Akıllı Hata Radarı sağ panelden alt çekmeceye taşındı; 3 sekmeli (`🚨 Hata & Uyarı Radarı`, `📜 Sistem & Git Logları`, `⚡ Son İşlemler`), daraltılabilir (`▼/▲`) ve log temizleme araçlı alt panel oluşturuldu.
- **Tam Genişlikte Hata & Uyarı Radarı Kartları (`Full-Width Radar Cards`):** Hata ve uyarı kartları küçük ızgara kutularından tam satır genişliğine yayılan zengin bilgi kartlarına dönüştürüldü. Etkilenen dosyalar, CI hata logu snippet'i, tek tıkla kopyalanabilir terminal komutu ve doğrudan çakışma stüdyosuna/temizliğe götüren aksiyon butonları eklendi.
- **🌱 Taze Yerel Dal Ayrımı (`collector.go`):** Henüz pushlanmamış `aheadMain == 0` yerel dalların yanlışlıkla `origin: gone` / silinmiş kalıntı sayılması kökten engellendi; `🌱 Yeni Yerel Dal` kimliği ve eylemleri atandı.
- **Mükerrer Dal Açma Kalkanı & Dinamik Dal Adı Üretimi:** Hem UI'da hem API'de mevcut veya birleşmiş dal adlarıyla yeniden dal açılması engellendi; statik isim öneri hatası dinamik dosya bazlı (`feat/<dosya>-guncel`) öneriyle değiştirildi.
- **Pürüzsüz `git checkout -b` Stüdyo Akışı:** Çalışma ağacında kirli dosya varken dal açma `git switch -c ... origin/main` yerine güvenli yerel `git checkout -b` akışına geçirilerek GitArc içinden tek tıkla commit & PR açma hatasız hale getirildi.
- **Konsolsuz Dal Açma, Commit & PR Oluşturma Stüdyosu (`/api/working-tree/commit`):** Çalışma ağacındaki kirli değişiklikleri (`wip/working-tree` veya yerel dallar) terminal açmadan yeni özellik dalına alma, commit mesajı, PR başlığı ve PR açıklamasını düzenleyip yerelde commitleme veya tek tıkla push + PR açma desteği eklendi.
- **Push Öncesi PR Başlık & Açıklama Düzenleyici (`PromoteBranchWithMeta` + `/api/branch/promote`):** Alt raydaki yerel dalları `origin`'e pushlamadan önce PR başlığı ve açıklamasını kokpit içinden hazırlama ve bu bilgilerle PR açma eklendi.
- **Canlı `CI: RUNNING` Pulse Efekti & Akıllı Auto-Merge Kuyruğu:** `CI: RUNNING` durumundaki dal parabolü, rozetleri ve Inspector çipi animasyonlu nabız efektiyle (`ci-running` / `running-pulse`) vurgulanır; CI çalışırken Merge / `⏳ CI Bitince Merge Et` tıklandığında dal kuyruğa alınır, CI geçince otomatik merge edilir, başarısız olursa kuyruk iptal edilip hata bildirimi + kalıcı log üretilir.

## [0.7.0] - 2026-11-16

### Eklenenler
- **Mega-Repo (10K Geliştirici) Ölçekleme Altyapısı:** SHA-bazlı dal değerlendirme önbelleği (`branchEvalCache` + `CacheHits`), `--sort=-committerdate --count=40` dal sınırı, ters indeksli (`O(N * F)`) Cross-PR çarpışma motoru, **`👤 Benim Dallarım (@me)`** ve anlık **`📂 Klasör/Dosya Merceği`**.
- **Çapraz Platform Masaüstü & CI Doğrulaması:** `openBrowser` içine Windows (`rundll32`) desteği eklendi ve `.github/workflows/ci.yml` içine `linux`/`darwin`/`windows` çapraz derleme adımları eklendi.
- **Team Server Modu (`-team`):** Paylaşılan çalışma dizini `HEAD`'ini değiştiren `Checkout` / `Amend` işlemlerini kilitler; izole `git worktree` çakışma çözümü, PR merge ve uzak dal yaşam döngüsünü çoklu ekip kullanımına güvenli şekilde açar.
- **Merge Queue (`Auto-Merge`), GitHub Branch Protection & Self-Update Merkezi:** Merge stratejisine `--auto` (Merge Queue) eklendi; `main` dalı koruma kontrolcüsü (`/api/repo/protect`) ve uzak Git tag denetimli güncelleme merkezi (`/api/system/update-check`) eklendi.

## [0.6.0] - 2026-11-16

### Eklenenler
- **Kalıcı Sistem & Hata Günlüğü (`📜 Loglar` + `/api/logs/clear`):** 3 saniyede kaybolan toast yerine backend ve istemci taraflı tüm hata ve işlem kayıtları saat, dal ve komut ayrıntısıyla `CockpitSnapshot.Logs` içinde saklanır; üst çubuktaki `📜 Loglar` modalından tek tıkla kopyalanabilir veya temizlenebilir. Toast bildirimleri de tıklanarak kopyalanabilir hale getirildi.
- **Stüdyo İçi Kalıcı & Kopyalanabilir Hata Kutusu (`#studio-error-banner`):** Çakışma Çözüm Stüdyosunda `go vet` veya `git` hatası oluşursa kaybolmayan hata kutusu ve **`📋 Hatayı Kopyala`** butonu çıkar.
- **Akıllı Çakışma Rehberi (`★ ÖNERİLEN` + Otomatik Kod Riski Analizi):** Çakışan dosyanın `.go` kaynak kodu mu yoksa `CHANGELOG.md` / kilit dosyası mı olduğunu analiz ederek Türkçe **"💡 Ne Yapmalıyım?"** açıklaması sunar; kaynak kodda *Union* riskine karşı **`⚠ Kodda Çift Tanım / Hata Riski`** uyarısı verir, **`2. 🛡️ Main'deki Güncel Kodu Kullan`** seçeneğini önerir.

## [0.5.0] - 2026-11-16

### Eklenenler
- **Çakışma Çözüm Stüdyosu (Sol: Olan / Sağ: Olacak + 4 Adımlı Editör):** Çakışmalı canlı/simüle tüm dallar için `/api/conflict/inspect` ve yan yana stüdyo modalı; 3 yollu gerçek `git merge-file -p` bloklarını çıkarır ve tek tıkla **1. Union**, **2. Main (`ours`)**, **3. Dal (`theirs`)**, **4. 🤖 AI / Elle Düzenle** seçeneklerini canlı önizler.
- **Zero-Touch İzole `git worktree` + `go vet` Kilidi:** `ResolveBranchConflict` kullanıcının aktif çalışma dizinindeki kirli dosyalara hiç dokunmadan geçici `git worktree` içinde çözümü uygular, `go.mod` varsa `go vet ./...` ile derlemeyi doğrular ve temizse commit + `origin` push yapar.
- **Antigravity Doğrudan MCP Eklentisi & `gitarc_resolve_conflict`:** `.agents/plugins/gitarc/` eklentisiyle GitArc MCP sunucusu Antigravity'ye doğrudan bağlanır; `gitarc_check_conflicts` artık odaklı `conflictHunks` penceresi döndürür ve `gitarc_resolve_conflict` MCP üzerinden tek adımda çözüm uygular.
- **MCP Bağlantı Merkezi Modalı (`🔌 MCP Bağla`):** Antigravity, Claude Code ve Cursor için tek tıkla kopyalanabilir `mcp_config.json` penceresi eklendi.

## [0.4.0] - 2026-11-16

### Eklenenler
- **Uzak (`origin/*` / GitHub-Only PR) Dal Görünürlüğü & Çakışma Tespiti:** Yerel kopyası silinmiş olsa dahi `refs/remotes/origin/*` veya açık GitHub PR'ı bulunan tüm dallar üst yarı-kürede keşfedilir; `git merge-tree --write-tree` ile `origin/main` çakışmaları çalışma dizinine dokunmadan tespit edilir (`collectRemoteOnlyBranches`).
- **Tek Tıkla Uzak PR Kapatma & Remote Dal Silme / İndirme:** `DeleteBranch` ve `PruneGone` artık yalnızca yerel dalları değil `gh pr close --delete-branch` ve `git push origin --delete` ile uzak kalıntı ve eski PR dallarını da temizler; yerelde olmayan uzak dallar `git switch --track` ile tek tıkla yerelde indirilip geçiş yapılabilir.
- **TDD Mutasyon Test Süiti & Branch Adı Ayrıştırma Düzeltmesi:** `TestCollectLiveRepoRemoteOnlyAndConflictMutations` ile bare `origin` + klon üzerinde çakışma ve artık uzak dal mutasyonları doğrulanır; `reMergePRSubject` desenindeki `/` içeren dal adlarının kesilme hatası düzeltildi.

## [0.3.0] - 2026-11-16

### Eklenenler
- **Gömülü MCP Sunucusu (`./gitarc -mcp`):** Sıfır dış bağımlılıklı `stdio` JSON-RPC 2.0 sunucusu (`internal/mcp/server.go`); `gitarc_status`, `gitarc_check_conflicts`, kota dostu `gitarc_ci_failure_digest` ve `gitarc_sync_main` araçları.
- **PR Başlık/Açıklama & Son Commit Mesajı Düzenleme:** Açık PR'lar için `gh pr edit` (`/api/pr/edit`), aktif açık özellik dalının (`HEAD`) son commit'i için `--only` ve `--force-with-lease` korumalı `AmendLastCommit` (`/api/commit/amend`); `main` ve eski commitler kilitli.
- **Tek Tıkla Dala Geçiş (`Checkout`) & Merge Stratejisi:** `HEAD` olmayan dallar için `git switch` (`/api/branch/checkout`), `Merge | Squash | Rebase` strateji seçimi ve sağ panelde **"📋 Tek Tıkla Terminal Komutu"** kutusu.
- **Görsel Hiyerarşi & Canlı Motor İyileştirmeleri:** Birleşmiş kemerler için kompakt `.cap-merged-pill` rozetleri, `origin/main` bazlı `git merge-tree` & diff hesaplamaları, 10 sn `gh` CLI önbelleği, squash-merge dahil güvenli temizlik (`git branch -D`), kalıcı depo hafızası (`localStorage`) ve 4 sn otomatik yenileme.

## [0.2.0] - 2026-11-16

### Eklenenler
- **Canlı Dogfooding & Yaşam Döngüsü Aksiyonları:** Canlı modda gerçek `git`/`gh` komutlarıyla dal açma (`CreateBranch`), alt yarı-küreden push & PR açma (`PromoteBranch`), PR/yerel merge (`MergeBranch`), `origin: gone` temizliği (`PruneGone`) ve `main` senkronizasyonu (`SyncSpine`).
- **Derin GitHub Actions & Merge Kemer Entegrasyonu:** `gh run view <id> --log-failed` ile canlı CI hata logu ayıklama, `gh pr list --state merged` ile squash/rebase birleşmiş PR kemerlerini çizme ve `%(upstream:track)` ayrıştırma.
- **Güvenlik Sertleştirmesi:** Tüm dal adları için `validateBranchName` (`-` öneki ve geçersiz ref koruması).

## [0.1.0] - 2026-11-16

### Eklenenler
- **Parabolik SVG Ray Sistemi:** Merkez `main` omurgası üzerinde `origin/main` ↑/↓ farkı, üst/alt yarı-küre ayrımı, Viyadük S-Ray ve Saf Parabol geometri modları ile deterministik Interval Lane Packing (`internal/gitengine/lanes.go`).
- **Birleşik Dal Kapsülleri (Branch Pair Capsules):** Üst katmanda PR, Review ve CI durumu; alt katmanda yerel dal adı, `HEAD` ve uncommitted dosya sayacı.
- **Akıllı Teşhis Motoru:** Çalışma dizinine dokunmadan `git merge-tree --write-tree` çakışma tespiti, `CHANGELOG.md` / lockfile için `Auto-Resolve`, Cross-PR dosya çarpışma radarı ve gürültüsüz Go test/panic CI log ayıklayıcı.
- **KeyLab Güvenlik & Embed Sunucusu:** `127.0.0.1` dinleme, `HttpOnly; SameSite=Strict` oturum çerezi, `X-Requested-With: gitarc` CSRF koruması, katı CSP ve `LocalFallbackEngine`.
