# WERD Panel

Panel desktop untuk lingkungan pengembangan web lokal di Windows — Apache, MySQL, PHP, phpMyAdmin, dan Composer dalam satu aplikasi dengan tray icon.

![WERD Panel](ss.png)

## Fitur

- **Apache 2.4.66 + PHP 8.4.24** (mod_php) · HTTP :80 dan HTTPS :443
- **MySQL 8.4** (opsional, bisa diunduh on-demand dari aplikasi) · port 3306
- **phpMyAdmin 5.2.3** di `/phpmyadmin` (login otomatis sebagai root)
- **Composer 2.10.2** via CLI `bin/composer-2.10.2/composer.bat`
- Kelola project web lokal: tambah path project, buka langsung di browser
- **Custom domain + SSL** per project (mkcert) — sertifikat trusted, hosts file diedit otomatis
- Start / stop / restart semua service, kontrol dari tray icon
- Autostart saat Windows login, edit file config langsung dari aplikasi

## Cara Pakai

1. Ekstrak zip sehingga `bin` berada di `E:\werd\bin`.
2. Jalankan `werd.exe` — Windows akan meminta izin **Administrator** (dibutuhkan untuk mengelola service dan hosts file).
3. Klik **Start** di panel untuk menjalankan semua service.

## Build

```sh
wails build -platform windows/amd64 -clean -upx
```

Arsitektur file:

```
bin/                        runtime (Apache, MySQL, PHP, phpMyAdmin, Composer)
config/                     konfigurasi aplikasi (httpd.conf, config.inc.php, ...)
var/                        vhosts, log, database
werd.exe                    aplikasi utama
```