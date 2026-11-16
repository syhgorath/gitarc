# GitArc v0.3.0 — Mimari Şartname ve 11 Kurallık Sistem Planı

Bu belge GitArc'ın görsel parabolik kokpit, canlı Git/GitHub motoru, PR/commit mesaj düzenleme ve gömülü MCP sunucusu için kesinleşmiş mimari kurallarını içerir.

## 1. Kesin Mimari Kurallar ("Olur / Olmaz / Şöyle Olur")

1. **Merkez Referans (`origin/main`):**
   - `HEAD` hangi dalda olursa olsun `git fetch --prune origin main:main` (eğer `HEAD == main` ve çalışma dizini temizse `git merge --ff-only origin/main`) ile yerel `main` çalışma dizini bozulmadan eşitlenir.
   - `ahead/behind`, `diff --name-only` ve `git merge-tree --write-tree` kontrolleri mevcutsa `origin/<defaultBranch>` bazlı hesaplanır.
2. **Üst vs. Alt Ray Ayrımı:**
   - Açık PR'ı olmayan veya pushlanmamış yerel dallar **Alt Rayda (kesikli mor yarım parabol)** açılır.
   - GitHub'da PR'ı `MERGED` olan ve yerelde yeni ek commit'i olmayan (`aheadMain == 0` / `remoteAhead == 0`) kalıntı dallar alt rayı kirletmez; **Birleşmiş / Temizlenebilir** olarak üst kemer / prune adayı şeklinde görünür.
   - Açık PR'ı olan bir dalda pushlanmamış yerel commit veya uncommitted değişiklik varsa, alt rayda eşlik eden bir `local-wip-<dal>` yarım parabolü gösterilir.
3. **Görsel Hiyerarşi (Kemer vs. Kapsül):**
   - **Birleşmiş kemerler (`merged`):** Tepe noktalarında büyük kart yerine kompakt kemer rozeti (`188×36px`: `✓ #PR dal-adı` + `🧹`) durur.
   - **Açık yarım paraboller (`open`):** Tam boy aksiyonlu Kapsül Kart taşır ve yatayda `264px` aralıkla çakışmadan dizilir.
4. **Canlı Depo Hafızası & 4 Saniyelik Otomatik Takip:**
   - `./gitarc` bir Git deposu içinde çalıştırıldığında varsayılan olarak doğrudan o canlı depoyla (`-repo .`) açılır; seçilen depo `localStorage`'da (`gitarc_repo_id`) saklanır.
   - `gh` CLI sorguları `repoPath` bazlı 10 saniyelik bellek içi önbellekte (`ghCache`) tutulur ve mutasyon sonrası temizlenir; arayüz 4 saniyede bir `/api/snapshot` yoklar.
5. **Squash-Merge Dahil Güvenli Dal Silme & Prune:**
   - Canlı modda önce `git branch -d` denenir; dalın PR'ı GitHub'da `MERGED` ise ve pushlanmamış yerel commit'i yoksa (`remoteAhead == 0`), squash-merge kaynaklı `not fully merged` engelini aşmak için `git branch -D` ile güvenle temizlenir.
6. **PR Başlık/Açıklama & Son Commit Mesajı Düzenleme:**
   - **PR Başlık & Açıklama:** Açık PR'lar için `gh pr edit <PR> --title ... --body ...` ile güncellenir.
   - **Son Commit Mesajı (`Amend`):** Yalnızca aktif dalın (`IsHEAD`) açık özellik dalındaki en son commit'i için `git commit --amend --only -m ...` çalıştırılır (`--only` staged/unstaged dosyaları karıştırmaz). Pushlanmış dallarda güvenli `git push --force-with-lease` uygulanır. `main` omurgasındaki ve eski ara commitlerde mesaj düzenleme **kilitlidir**.
7. **Gömülü MCP Sunucusu (`./gitarc -mcp`):**
   - Sıfır harici bağımlılıkla `stdio` JSON-RPC 2.0 sunucusu (`internal/mcp/server.go`):
     - `gitarc_status`: Topoloji, açık/birleşmiş dallar ve teşhis özeti.
     - `gitarc_check_conflicts`: `git merge-tree --write-tree` ve Cross-PR kesişim raporu.
     - `gitarc_ci_failure_digest`: Kota dostu regex-ayıklanmış CI hata özeti ve repro komutu.
     - `gitarc_sync_main`: Çalışma dizinini bozmadan `origin/main` eşitleme.
   - MCP üzerinden otomatik merge / dal silme / force-push **yasaktır**.
8. **KeyLab Güvenlik & Sıfır Bağımlılık:**
   - Go standart kütüphanesi, `127.0.0.1` dinleme, `HttpOnly; SameSite=Strict` oturum çerezi, `X-Requested-With: gitarc`, katı CSP (`style-src-attr 'none'`), `validateBranchName` ref koruması.
9. **Tek Tıkla Dala Geçiş (`Checkout`):**
   - `HEAD` olmayan her açık dalda `POST /api/branch/checkout` (`git switch <dal>`) aksiyonu sunulur.
10. **PR Birleştirme Stratejisi (`Merge | Squash | Rebase`):**
    - Merge işlemi `strategy` (`merge`, `squash`, `rebase`) parametresi destekler (`gh pr merge <no> --<strategy> --delete-branch`).
11. **Tek Tıkla Terminal Komutu Kutusu:**
    - Her seçili dal için durumuna özel tek tıkla kopyalanabilir `git`/`gh` komut zinciri (`suggestedCmd`) üretilir.

## 2. REST API Uçları

- `GET /api/snapshot`
- `POST /api/repo/switch`
- `POST /api/branch/create`
- `POST /api/branch/checkout`
- `POST /api/branch/merge` (`branchId`, `strategy`)
- `POST /api/branch/promote`
- `POST /api/branch/resolve`
- `POST /api/branch/fix-ci`
- `POST /api/branch/delete`
- `POST /api/branch/prune`
- `POST /api/pr/edit` (`branchId`, `title`, `body`)
- `POST /api/commit/amend` (`branchId`, `message`)
- `POST /api/spine/sync`
- `POST /api/simulation/reset`
- `POST /api/test/run`
