# Değişiklik Günlüğü (CHANGELOG)

Bu proje [Keep a Changelog](https://keepachangelog.com/tr/1.0.0/) biçimini ve SemVer sürümlemeyi izler.

## [0.8.0] - 2026-11-17

### Eklenenler
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
