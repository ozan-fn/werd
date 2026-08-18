# AGENTS.md

## Lokasi

- `bin/` — runtime lokal: httpd-2.4.66, php-8.4.23, mariadb-12.3.2, phpMyAdmin-5.2.3
- Root proyek: `E:\werd`

## Aturan Keras

- **DILARANG** mengubah/menambah/menghapus apa pun di dalam `bin/`. File di
  `bin/` (httpd, PHP, MariaDB, phpMyAdmin) adalah vendor yang diunduh; jangan diedit.
- Konfigurasi aplikasi ditaruh di luar `bin/` (root proyek), bukan di folder vendor.
- `run.sh` menyalin konfigurasi dari `config/` ke `bin/` saat start.

## Lingkungan

- Platform: Windows (win32), shell: bash
- Server: Apache (`bin/httpd-2.4.66-251206-Win64-VS17/Apache24/bin/httpd.exe`)
- PHP: `bin/php-8.4.23-Win32-vs17-x64` (mod_php, `php8apache2_4.dll`)
- MariaDB: `bin/mariadb-12.3.2-winx64`
- Untuk menjalankan server: `./run.sh`, berhenti: `./stop.sh`

## Konvensi

- Tanpa komentar kecuali diminta.
- Edit file yang ada; jangan menambah file/fungsi tanpa perlu.