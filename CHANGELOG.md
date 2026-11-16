# Değişiklik Günlüğü (CHANGELOG)

Bu proje [Keep a Changelog](https://keepachangelog.com/tr/1.0.0/) biçimini ve SemVer sürümlemeyi izler.

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
