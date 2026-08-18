#!/usr/bin/env bash
ROOT="E:/werd"
HTTPD="$ROOT/bin/httpd-2.4.66-251206-Win64-VS17/Apache24"
PHP_DIR="$ROOT/bin/php-8.4.23-Win32-vs17-x64"
MDB="$ROOT/bin/mysql-8.4.11-winx64"
mkdir -p "$ROOT/var/logs"

cp "$ROOT/config/httpd.conf" "$HTTPD/conf/httpd.conf"
cp "$ROOT/config/php.ini" "$PHP_DIR/php.ini"
cp "$ROOT/config/my.ini" "$MDB/my.ini"

if [ ! -d "$ROOT/var/mysql/mysql" ]; then
    mkdir -p "$ROOT/var/mysql"
    "$MDB/bin/mysqld --initialize-insecure" --datadir="$ROOT/var/mysql" >"$ROOT/var/logs/db-init.log" 2>&1
fi

"$MDB/bin/mysqld.exe" --defaults-file="$MDB/my.ini" >"$ROOT/var/logs/mysql.log" 2>&1 &
disown
sleep 3
"$HTTPD/bin/httpd.exe" -d "$HTTPD" >"$ROOT/var/logs/apache.log" 2>&1 &
disown

echo
echo "phpMyAdmin : http://127.0.0.1:8080"
echo "Stop       : ./stop.sh"