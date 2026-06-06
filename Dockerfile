# 1. Aşama: Derleme (Build Stage)
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Node.js kurulumu (Tailwind CSS derlemesi için)
RUN apk add --no-cache git nodejs npm

# Bağımlılıkları kopyala ve yükle
COPY go.mod go.sum ./
RUN go mod download

# Tüm kodları kopyala
COPY . .

# Tailwind CSS derle
RUN npm install && npm run build:css

# Uygulamayı optimize edilmiş şekilde derle (CGO olmadan, templates embedded)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o gotree main.go

# 2. Aşama: Çalışma Ortamı (Runtime Stage)
FROM alpine:3.19

WORKDIR /app

# Zaman dilimi ve SSL sertifikalarını kur
RUN apk add --no-cache ca-certificates tzdata

# Derlenen uygulamayı kopyala (templates/static embed edilmiş)
COPY --from=builder /app/gotree .

# SQLite veritabanı ve profil resimleri için kalıcı dizinler oluştur
RUN mkdir -p /app/data /app/uploads

# Ortam Değişkenleri
ENV PORT=1907
ENV DB_PATH=/app/data/gotree.db

# Portu dışarı aç
EXPOSE 1907

# Veri kaybını önlemek için volume tanımlamaları
VOLUME ["/app/data", "/app/uploads"]

# Uygulamayı başlat
CMD ["./gotree"]
