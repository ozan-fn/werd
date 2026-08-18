# WERD Panel

Alternatif XAMPP/Laragon: panel kontrol Apache + PHP + MariaDB + phpMyAdmin
untuk Windows.

## Isi Archive (`werd-windows-x64.zip`)

Extract sehingga `bin` berada di `E:\werd\bin` (path konfigurasi bersifat
absolut `E:/werd/...`).

```
werd/
├── bin/                                  # runtime (diunduh saat build)
│   ├── httpd-2.4.66-251206-Win64-VS17/   # Apache 2.4.66
│   │   └── Apache24/
│   │       ├── bin/
│   │       │   ├── httpd.exe             # server Apache
│   │       │   └── ...
│   │       └── conf/httpd.conf           # disalin dari config/ saat start
│   ├── php-8.4.23-Win32-vs17-x64/        # PHP 8.4 (TS, mod_php)
│   │   ├── php8apache2_4.dll
│   │   ├── php.ini                       # disalin dari config/ saat start
│   │   └── ext/
│   └── phpMyAdmin-5.2.3-english/         # phpMyAdmin 5.2.3 (DocumentRoot)
├── config/                               # sumber kebenaran konfigurasi
│   ├── httpd.conf
│   ├── php.ini
│   └── my.ini
├── var/
│   ├── logs/                             # apache.log, mariadb.log, php_errors.log ...
│   └── mariadb/                          # datadir MariaDB (dibuat saat init)
├── web/
│   ├── dist/                             # build frontend (+ .br brotli)
│   └── src/                              # source dashboard Preact
├── main.go                               # backend: static + WebSocket + start/stop
├── werd.exe                              # biner server panel (port 8090)
├── run.sh                                # mulai Apache + MariaDB + salin config
├── stop.sh                               # hentikan Apache + MariaDB
└── .gitkeep
```

## Menjalankan

```bash
./run.sh                 # start Apache + MariaDB + salin config
./stop.sh                # stop Apache + MariaDB
go run .                 # atau pakai werd.exe
```

- Panel kontrol : http://127.0.0.1:8090
- phpMyAdmin   : http://127.0.0.1:8080
- MariaDB      : port 3306

`werd.exe` menyajikan dashboard, WebSocket `/ws`, kontrol start/stop per
service, dan log real-time (mirip XAMPP).
