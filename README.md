# GoTREE: Self-Hosted Minimalist Linktree Alternatif Açık Kaynak Biyografi Platformu

Biolink, Linktree benzeri, minimalist tasarımlı, ultra hızlı ve sunucuda **25MB RAM altında çalışacak şekilde optimize edilmiş** self-hosted bir biyografi platformudur. Tüm statik varlıkları (`templates` ve `static` dosyaları) derleme aşamasında binary içerisine gömdüğü (`go:embed`) için tek bir dosya halinde taşınabilir ve anında yayına alınabilir (Deploy-ready).

---

## ⚡ Temel Özellikler & Avantajlar

*   **Aşırı Düşük Kaynak Tüketimi:** Standart Go HTTP kütüphanesi ve optimize SQLite altyapısı sayesinde 25MB RAM sınırının altında yüksek performansla çalışır.
*   **Gerçek Zamanlı WYSIWYG Düzenleyici:** Yönetim panelinde yapılan tüm değişiklikler (başlık, açıklama, renkler, stil, ikon sıralaması) sağdaki telefon simülatöründe **anında (sayfa yenilenmeden)** görüntülenir.
*   **Mobil Önizleme Desteği:** Mobil cihazlardan giren yöneticiler için sağ altta yüzen bir "Önizleme" butonu ve buna bağlı blur efektli özel bir mobil simülatör modalı sunulmuştur.
*   **11 Hazır Premium Tema:**
    *   *Tatiana Reine (Sunset Glow)*, *Erik Susanna (Cosmic Space - Dinamik CSS Yıldızlı)*, *Sticky Wicks (Çikolatalı Krem)* temaları dahil olmak üzere toplam 11 adet harici bağımlılıksız şık şablon.
*   **Gelişmiş Buton Stili Özelleştirme:**
    *   Klasik, Neo-Brutalist, Metalik, Cam (Glassmorphism), Neon Işıltılı, Modern Gölgeli, Sarı Siberpunk, Parlayan Kenarlıklı, Yumuşak Kil (Neumorphism), Retro Disket, Renkli Işıltılı Gradiyent, Kuzey Işıkları Buzlu Camı, Minimalist Düz Kart ve Kenarlıklı Buzlu Cam dahil 16 farklı buton stili.
*   **Arama Yapılabilir Lucide İkon Kütüphanesi:** Kompakt arama arayüzü sayesinde 100+ popüler Lucide ikonu arasından saniyeler içinde seçim yapıp bağlantılara ekleme.
*   **Gelişmiş Ziyaretçi İstatistikleri (Chart.js):** IP adresinden ülke tespiti yapamayan yerel / intranet ortamlar için akıllıca dil çıkarımlı fallback mekanizmalı ziyaret ve tıklama grafik paneli.
*   **Yapay Zeka (AI Agent) Dostu REST API & Docs:** Biolink'i yapay zeka ajanlarına veya otomasyon sistemlerine bağlamak için `X-API-Key` korumalı API uç noktaları ve `/api/docs` altında etkileşimli dökümantasyon.

---

## 📂 Dosya Yapısı (Project Structure)

```text
├── db/                 # SQLite veritabanı ilklendirme, şema ve göç işlemleri
├── handlers/           # HTTP Handler'ları (Autosave, API rotaları, sayfa render işlemleri)
├── middleware/         # Auth, Cookie ve X-API-Key doğrulaması yapan ara katmanlar
├── static/             # CSS, JS ve harici kütüphaneler (Gömülü / Embedded)
│   ├── dist/           # Derlenmiş Tailwind CSS dosyaları
│   ├── js/             # Alpine.js, Lucide, Chart.js kütüphaneleri
│   └── src/            # Kaynak CSS dosyaları
├── templates/          # Go HTML Şablonları (Gömülü / Embedded)
│   ├── layout.html     # Genel sayfa HTML iskeleti
│   ├── admin.html      # Yönetim paneli arayüzü (Reaktif Alpine & Canlı Önizleme)
│   ├── bio.html        # Ziyaretçilerin gördüğü biyografi sayfası
│   └── login.html / setup.html
├── uploads/            # Yüklenen profil resimleri ve arka plan görselleri (Kalıcı Klasör)
├── Dockerfile          # Optimize Multi-stage Linux Build yönergesi
├── go.mod / go.sum     # Go bağımlılık yöneticileri
├── package.json        # Tailwind derleme scriptleri
└── main.go             # Sunucu giriş noktası ve yönlendirme tanımları
```

---

## 🛠️ Yapılandırma Seçenekleri (Port & Veritabanı)

Biolink'in varsayılan çalışma portu **1907**'dir. Sistem hem **Komut Satırı Parametrelerini (Flags)** hem de **Ortam Değişkenlerini (Env Variables)** destekler:

### 1. Komut Satırı Bayrakları
```bash
# Portu 8080 yapmak ve veritabanını başka bir klasöre taşımak için:
./gotree -port 8080 -db ./data/biyografi.db
```

### 2. Ortam Değişkenleri
*   `PORT`: Sunucunun dinleyeceği port (örn: `8080`)
*   `DB_PATH`: SQLite veritabanı dosyasının tam yolu (örn: `/var/data/gotree.db`)

---

## 🐳 Docker ile Kurulum (Deploy-ready)

Biolink, kalıcı depolama gereksinimleri (veri ve resimler) için iki adet volumeye ihtiyaç duyar.

### 1. Docker Compose ile Başlatma
Aşağıdaki `docker-compose.yml` dosyasını oluşturun:

```yaml
version: '3.8'

services:
  gotree:
    image: gotree:latest
    build: .
    container_name: gotree_app
    ports:
      - "1907:1907"                     # Dış Port:İç Port (Varsayılan 1907)
    environment:
      - PORT=1907
      - DB_PATH=/app/data/gotree.db
    volumes:
      - ./gotree_data:/app/data       # Kalıcı SQLite Veritabanı
      - ./gotree_uploads:/app/uploads # Kalıcı Profil ve Arka Plan Resimleri
    restart: unless-stopped
```

### 2. İmajı Derleyin ve Çalıştırın
```bash
docker compose up -d --build
```
Kurulum tamamlandıktan sonra `http://localhost:1907` adresinden ilk Super Admin kaydını yapabilirsiniz.

---

## 💻 Yerel Geliştirme ve Çalıştırma

### Gereksinimler
*   [Go 1.22+](https://go.dev/dl/)
*   [Node.js v18+](https://nodejs.org/) (Yalnızca Tailwind CSS derlemek için)

### 1. CSS Varlıklarını Derleyin
```bash
npm install
npm run build:css
```

### 2. Go Sunucusunu Derleyin ve Başlatın
```bash
# Derleme (Linux/macOS için)
go build -o gotree main.go

# Derleme (Windows için)
go build -o gotree.exe main.go

# Başlatma
./gotree
```
Sunucu başarıyla çalışmaya başladığında tarayıcınızdan ayarladığınız porta giderek kullanmaya başlayabilirsiniz.
