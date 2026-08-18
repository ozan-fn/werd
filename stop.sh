#!/usr/bin/env bash
set -e
ROOT="E:/werd"
MDB="$ROOT/bin/mysql-8.4.11-winx64/bin"
"$MDB/mysqladmin.exe" -u root shutdown 2>/dev/null || taskkill //IM mysqld.exe //F >/dev/null 2>&1
taskkill //IM httpd.exe //F >/dev/null 2>&1
echo "Stopped."