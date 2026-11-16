# GitArc Çalışma Alanı Kuralları

## Commit ve Pull Request İş Akışı (Kalıcı Kullanıcı Tercihi)

Kullanıcı bu projede her geliştirme, hata düzeltme veya özellik ekleme oturumunun sonunda Git ve GitHub işlemlerini kendisi tek tıkla (kopyala-yapıştır ile) çalıştırmak istemektedir:

- Değişiklikleri tamamladıktan ve testleri (`go vet ./... && go test -v ./...`) doğruladıktan sonra **otomatik commit veya push yapma** (kullanıcı açıkça istemedikçe).
- Bunun yerine her zaman yanıtın sonunda aşağıdaki kalıpta hazır, tek tıkla kopyalanabilir komut blokları ve açıklamalar sun:
  1. **Dal / Commit Komutu:** Önerilen branch adı (gerekirse `git checkout -b ...`), `git add`, anlamlı Türkçe/Conventional Commit başlıklı `git commit -m "..."` ve `git push` komutu.
  2. **PR Başlığı ve Açıklaması + `gh pr create` Komutu:** PR başlığı, neyin neden değiştiğini ve doğrulama adımlarını özetleyen gövde metniyle birlikte tek seferde çalıştırılabilir `gh pr create --title "..." --body "..."` komutu.
