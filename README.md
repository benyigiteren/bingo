# Bingo

Bingo; Go diliyle yazılmış, minimalist, yüksek performanslı ve "az RAM" felsefesini benimseyen self-hosted (kendi sunucunuzda barındırabileceğiniz) bir dosya ve metin paylaşım platformudur. Tarayıcınız üzerinden veya API/betikler aracılığıyla görselleri, belgeleri ve metinleri anında paylaşmanıza ve güvenli bir şekilde sunmanıza olanak tanır.

---

## 🚀 Temel Özellikler

* **Minimum Kaynak Tüketimi:** Tamamen Go standart kütüphanesiyle (harici router bağımlılığı olmadan) derlenmiş, CGO barındırmayan ve <15MB RAM kullanımıyla çalışan hafif mimari.
* **Gömülü SQLite (WAL Modu):** Veri tabanı dosyası `data/` altında tutulur; WAL (Write-Ahead Logging) modlu SQLite entegrasyonu sayesinde yüksek eşzamanlılıkta okuma/yazma performansı sunar.
* **Akıllı Dosya ve Metin Yönlendirmesi:**
  * **Görseller & JSON:** Doğrudan tarayıcıda ham haliyle gösterilir.
  * **Markdown (`.md`) & Düz Metin (`.txt`):** Tarayıcıda premium, monokrom bir okuyucu ekranında (`viewer`) gösterilir. `?raw=true` parametresiyle ham çıktı alınabilir.
  * **Dahili Metin Editörü:** Dosya yüklemeden tarayıcı üzerinden doğrudan Markdown veya düz metin yazıp paylaşmanızı sağlar.
* **Siber Güvenlik Önlemleri:**
  * **CSRF Koruması:** Web arayüzündeki tüm post işlemlerinde session-bound CSRF token doğrulaması yapılır.
  * **Stored XSS Engelleme:** Güvensiz web uzantıları (`.html`, `.svg`, `.js` vb.) tarayıcıda kod çalıştırmaması için zorunlu dosya indirme (`Content-Disposition: attachment`) yöntemiyle servis edilir.
  * **Rate Limiting:** IP bazlı genel hız sınırlaması ve API anahtarı bazlı özel hız sınırlaması (Token Bucket algoritması ile hafızada tutulur, leaksiz temizlenir).
* **Agentic AI ve API Uyumu:** Yapay zeka ajanları ve otomasyon betikleri için API Key desteği ile dosya, JSON gövdesi veya piped düz metin yükleme desteği.
* **Yönetici Kontrol Paneli:** Süper Yönetici tarafından yeni kullanıcı ekleme, kullanıcı aktif/pasif etme, API anahtarı yenileme ve yüklenen dosyaları izleyip silme paneli.
* **Monokrom Minimal Tasarım:** Light ve Dark sistem temalarına otomatik uyum sağlayan, göz yormayan, premium monokrom CSS mimarisi.

---

## 🛠️ Kurulum ve Çalıştırma

### Yöntem A: Docker Compose (Önerilen)

Docker kurulu olan makinenizde projeyi tek bir komutla ayağa kaldırabilirsiniz:

1. Depoyu klonlayın.
2. Aşağıdaki komut ile container'ları başlatın:
   ```bash
   docker-compose up -d
   ```
3. Tarayıcınızdan `http://localhost:8080` adresine gidin.
4. Karşınıza çıkacak **İlk Kurulum** ekranından ilk kullanıcıyı oluşturun. Bu kullanıcı **Süper Yönetici** (Super Admin) olur ve bu işlemden sonra dışarıdan üye kayıtları tamamen kilitlenir.

### Yöntem B: Go ile Yerel Derleme

Projeyi yerel makinenizde Go kullanarak çalıştırmak için:

1. Bağımlılıkları indirin:
   ```bash
   go mod download
   ```
2. Uygulamayı başlatın:
   ```bash
   go run main.go
   ```
3. Projeyi derleyip çalıştırmak isterseniz:
   ```bash
   go build -o bingo main.go
   ./bingo
   ```

Varsayılan konfigürasyonda platform `:8080` portundan çalışır, veri tabanı `./data/bingo.db` altında, yüklenen dosyalar ise `./uploads/` altında saklanır. Bu değerleri `PORT` ve `DB_PATH` ortam değişkenleri (Env) ile değiştirebilirsiniz.

---

## 📂 Proje Yapısı

```
bingo/
├── db/                  # SQLite veri tabanı şeması ve CRUD işlevleri
├── middleware/          # Oturum yönetimi, CSRF doğrulama ve Rate Limiter
├── handlers/            # Setup, Giriş, Dosya Paylaşım ve API İşleyicileri
├── templates/           # Türkçe HTML şablonları (Bento istatistikler, Editör, Viewer)
├── static/              # CSS ve JavaScript yardımcı dosyaları
├── Dockerfile           # Çok aşamalı (multi-stage) minimal Docker yapılandırması
├── docker-compose.yml   # Konfigüre edilmiş Docker Compose dosyası
└── .gitignore           # Derleme çıktıları ve verileri hariç tutan git yoksay dosyası
```

---

## 📡 API Kullanımı

API üzerinden paylaşım yapmak için panonuzdan alacağınız API anahtarını kullanmalısınız.

### 1. Dosya Yükleme (Multipart Form)
```bash
curl -X POST \
  -H "X-API-Key: bg_api_anahtariniz" \
  -F "file=@resim.png" \
  http://localhost:8080/api/upload
```

### 2. Metin Yükleme (JSON Gövdesi)
```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: bg_api_anahtariniz" \
  -d '{"text": "# Doküman\nMerhaba Bingo!", "filename": "not.md"}' \
  http://localhost:8080/api/upload
```

### 3. Ham Metin Yükleme (Pipe / Akış)
```bash
echo "Sistem log kayıtları..." | curl -X POST \
  -H "Content-Type: text/plain" \
  -H "X-API-Key: bg_api_anahtariniz" \
  --data-binary @- \
  http://localhost:8080/api/upload
```

---

## 📄 Lisans

Bu proje açık kaynaklıdır ve MIT lisansı altında dağıtılmaktadır. Dilediğiniz gibi geliştirebilir, çatallayabilir (fork) ve kendi sunucunuzda barındırabilirsiniz.
