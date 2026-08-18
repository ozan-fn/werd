#!/usr/bin/env bash
set -e
ROOT="E:/werd"
MDB="$ROOT/bin/mariadb-12.3.2-winx64/bin"
"$MDB/mariadb-admin.exe" -u root shutdown 2>/dev/null || taskkill //IM mariadbd.exe //F >/dev/null 2>&1
taskkill //IM httpd.exe //F >/dev/null 2>&1
echo "Stopped."