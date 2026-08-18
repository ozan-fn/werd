#!/usr/bin/env bash
ROOT="E:/werd"
HTTPD="$ROOT/bin/httpd-2.4.66-251206-Win64-VS17/Apache24"
PHP_DIR="$ROOT/bin/php-8.4.23-Win32-vs17-x64"
MDB="$ROOT/bin/mariadb-12.3.2-winx64"
mkdir -p "$ROOT/var/logs"

cp "$ROOT/config/httpd.conf" "$HTTPD/conf/httpd.conf"
cp "$ROOT/config/php.ini" "$PHP_DIR/php.ini"
cp "$ROOT/config/my.ini" "$MDB/my.ini"

if [ ! -d "$ROOT/var/mariadb/mysql" ]; then
    mkdir -p "$ROOT/var/mariadb"
    "$MDB/bin/mariadb-install-db.exe" --datadir="$ROOT/var/mariadb" >"$ROOT/var/logs/db-init.log" 2>&1
fi

"$MDB/bin/mariadbd.exe" --defaults-file="$MDB/my.ini" >"$ROOT/var/logs/mariadb.log" 2>&1 &
disown
sleep 3
"$HTTPD/bin/httpd.exe" -d "$HTTPD" >"$ROOT/var/logs/apache.log" 2>&1 &
disown

echo
echo "phpMyAdmin : http://127.0.0.1:8080"
echo "Stop       : ./stop.sh"