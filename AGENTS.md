# GitArc Çalışma Alanı Kuralları ve Mühendislik Standartları

> Yeni veya daha ekonomik modeller bu projede çalışırken önce kök dizindeki `HANDOVER.md` dosyasını okumalıdır.

## 1. Commit ve Pull Request İş Akışı (Kalıcı Kullanıcı Tercihi)

Kullanıcı bu projede her geliştirme, hata düzeltme veya özellik ekleme oturumunun sonunda Git ve GitHub işlemlerini kendisi tek tıkla (kopyala-yapıştır ile) çalıştırmak istemektedir:

- Değişiklikleri tamamladıktan ve doğrulama komutunu (`go vet ./... && go test -count=1 -v ./... && go build -o gitarc .`) çalıştırdıktan sonra **otomatik commit veya push yapma** (kullanıcı açıkça istemedikçe).
- Bunun yerine her zaman yanıtın sonunda aşağıdaki kalıpta hazır, tek tıkla kopyalanabilir komut blokları ve açıklamalar sun:
  1. **Dal / Commit Komutu:** Önerilen branch adı (`git checkout -b ...`), `git add`, anlamlı Conventional Commit başlıklı `git commit -m "..."` ve `git push -u origin ...` komutu.
  2. **PR Başlığı ve Açıklaması + `gh pr create` Komutu:** PR başlığı, neyin neden değiştiğini ve doğrulama adımlarını özetleyen gövde metniyle tek seferde çalıştırılabilir `gh pr create --title "..." --body "..."` komutu.

## 2. Kırmızı Çizgiler (Asla İhlal Etme)

1. **Sıfır Dış Bağımlılık (`go.mod`):** Projede yalnızca Go standart kütüphanesi (`stdlib`) ve `//go:embed web` kullanılır. Harici Go paketi, `npm`/`node_modules` veya CDN linki ekleme.
2. **Sıkı CSP (`style-src-attr 'none'`):**
   - `web/index.html` ve `web/app.js` içinde **ASLA** satır içi `style="..."` veya `style='...'` özniteliği yazma (`el.style.*` ataması da yapma).
   - `web/*.html` içinde harici `http://` veya `https://` URL kullanma (`http://www.w3.org/2000/svg` SVG XML namespace hariç).
   - Tüm görsel durumları `web/tasarim.css` içinde CSS sınıfı olarak tanımla (`hidden`, `.running-pulse`, `.ci-running`, `.ci-fail` vb.). `main_test.go` içindeki `TestWebAssetsStrictCSPCompliance` bunu her `go test` koşusunda zorunlu denetler.
3. **Üçlü Motor Eşliği (Tri-Mode Parity):** Eklenen her yeni özellik veya aksiyon şu 3 yerde birden çalışmalıdır:
   - **Canlı Git/GitHub Motoru:** `internal/gitengine/collector.go` + `internal/gitengine/simulation.go` (`*Live` metotlar)
   - **Sunucu Simülasyon Motoru:** `internal/gitengine/simulation.go` (`simRepo` bellek-içi durumu)
   - **Tarayıcı Offline Yedek Motoru:** `web/app.js` içindeki `LocalFallbackEngine`
4. **Zero-Touch Çalışma Dizini Kuralı:** Çakışma algılama (`git merge-tree --write-tree`) ve çakışma çözümü (`ResolveBranchConflict` geçici izole `git worktree`) kullanıcının aktif çalışma ağacındaki (`HEAD`) kirli dosyalara asla dokunmamalıdır. Çözüm pushlanmadan önce `go.mod` varsa `go vet ./...` doğrulamasından geçirilmelidir.
5. **Team Server Modu (`-team`):** `m.IsTeamMode()` aktifken paylaşılan `HEAD`'i değiştiren işlemler (`CheckoutBranch`, `AmendLastCommit`, `CommitWorkingTree`) engellenir; izole `worktree` çözümü, uzak dal silme/merge işlemleri çalışmaya devam eder.
6. **Güvenlik Kalkanları (`main.go`):** `Host` (DNS Rebinding), `gitarc_session` (`HttpOnly`, `SameSite=Strict`), `POST` isteklerinde `X-Requested-With: gitarc` + `Origin` kontrolü ve `validateBranchName`/`sanitizeID` girdi doğrulamaları korunmalıdır.
7. **Binary Yeniden Derleme:** `web/` dosyaları `//go:embed web` ile ikiliye gömülür; her UI veya Go değişikliği sonrası `go build -o gitarc .` çalıştırılmalıdır.
8. **Hata Geçirmezlik Prensibi (Foolproof UX & Otomatik Ön Koşul Çözümü):**
   - **Kullanıcıyı Çıkmaz Sokağa Sokma:** Bir aksiyon öncesinde Git ön koşulu gerekiyorsa (örn. silinecek dalın aktif `HEAD` olması, PR açılacak dalda kaydedilmemiş kirli dosyaların bulunması) çalışma ağacı temiz/güvenli olduğu sürece kullanıcıya *"önce başka dala geçin"* veya *"önce commit atın"* hatası döndürme; arka planda güvenli ön adımı (`git switch main`, `git add -A && git commit`) otomatik uygula.
   - **Benzersiz Varsayılan Öneriler:** Kullanıcıya dal veya etiket adı önerirken açık ve birleşmiş (`merged`/`gone`) tüm dalları denetle (`suggestUniqueBranchName`); daha önce birleşmiş veya mevcut bir dal adını asla tekrar varsayılan olarak önerme.
   - **Uzak Sunucuya Boş/Bozuk İstek Gönderme:** `gh pr create` veya `git push` öncesi `AheadMain == 0` ya da `go vet ./...` doğrulamasını yerelde çalıştır; ham GraphQL/CLI hatası sızdırmak yerine ne yapılması gerektiğini açıklayan net Türkçe uyarı döndür.
   - **Görsel Dürüstlük:** Eylemi olmayan öğede gereksiz buton gösterme; yalnızca yerel kalıntısı duran birleşmiş parabollerde `💻 Local` / `HEAD·Local` rozeti, alt ray kalıntı kuyruğu ve `🧹` butonu göster, tamamen temiz yeşil kapsüllerde `🧹` butonunu gizle.

## 3. Mimari ve Dosya Haritası (`v0.8.0`)

- `main.go` / `main_test.go`: HTTP sunucusu, güvenlik middleware (`root`, `guard`), 20 `/api/*` uç noktası, `-addr`, `-repo`, `-open`, `-mcp`, `-team` bayrakları, CSP & API güvenlik testleri.
- `internal/model/model.go`: `CockpitSnapshot`, `RepoMeta`, `CommitNode`, `BranchArc`, `PRStatus`, `CIStatus`, `ConflictDetail`, `ConflictStudioPayload`, `ActionLogEntry`.
- `internal/gitengine/collector.go`: `AppVersion` (`0.8.0`), `CollectLiveRepo`, SHA-bazlı `evalCache`, 40-dal sıralı penceresi, `collectLocalBranches`, `collectRemoteOnlyBranches`, `ParseMergeTreeOutput`, `BuildSuggestedCommand`.
- `internal/gitengine/simulation.go`: `Manager`, in-memory `simRepo` sanal durum yönetimi, `SyncSpine`, `ProtectDefaultBranch`, `CheckForUpdates`, kalıcı `actionLogs`.
- `internal/gitengine/live_mutations.go`: Canlı Git/GitHub mutasyonları (`commitWorkingTreeLive`, `promoteBranchLive`, `mergeBranchLive`, `deleteBranchLive` & `pruneGoneLive` otomatik `HEAD → main` geçişi, `updatePRMetaLive`, `amendLastCommitLive`).
- `internal/gitengine/conflict_studio.go`, `diagnostics.go`, `lanes.go`: İzole 3-yollu `git merge-file` stüdyosu ve akıllı öneri, ters indeks Cross-PR çarpışma ve CI log ayıklama, interval packing şerit yerleşimi.
- `internal/gitengine/engine_test.go`: Gerçek geçici bare/clone Git depolarıyla mutasyon, boş dal / otomatik `HEAD` temizliği ve iş akışı doğrulamaları dahil 17 test.
- `internal/mcp/server.go` & `server_test.go`: Sıfır bağımlılıklı `stdio` JSON-RPC 2.0 MCP sunucusu (`gitarc_status`, `gitarc_check_conflicts`, `gitarc_ci_failure_digest`, `gitarc_sync_main`, `gitarc_resolve_conflict`).
- `web/index.html`, `web/tasarim.css`, `web/fallback.js`, `web/app.js`: Parabolik SVG kokpit, alt ray yerel kalıntı kuyruğu (`.svg-local-remnant-path` / `.cap-local-remnant`), Sağ Inspector (`suggestUniqueBranchName`), `CI: RUNNING` nabız + `pendingAutoMerge` kuyruğu, modal stüdyolar ve modüler `LocalFallbackEngine` (`fallback.js`).

## 4. Zorunlu Doğrulama Adımları (Her Görev Sonu)

Değişiklik yaptıktan sonra yanıt vermeden önce mutlaka şu iki komutu çalıştırıp tüm testlerin sıfır hatayla geçtiğini gör:
```bash
go vet ./... && go test -count=1 -v ./... && go build -o gitarc .
node --check web/fallback.js && node --check web/app.js
```
Ayrıca yeni eklediğin her Go iş mantığı veya bugfix için `engine_test.go`, `main_test.go` veya `server_test.go` içinde otomatik test yaz.
