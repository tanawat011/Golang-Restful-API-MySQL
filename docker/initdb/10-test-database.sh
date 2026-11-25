#!/bin/sh
# Runs once, when the MySQL volume is first created: a second database for
# the tests (they empty its users table), so they never touch DB_NAME's data.
mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<SQL
CREATE DATABASE IF NOT EXISTS \`${MYSQL_DATABASE}_test\`;
GRANT ALL ON \`${MYSQL_DATABASE}_test\`.* TO '${MYSQL_USER}'@'%';
SQL
