<?php
declare(strict_types=1);

$cfg['blowfish_secret'] = 'X1Wq4!zR8@vP3#mN6$tL9%bH2&kD7*c';

$i = 0;
$i++;
$cfg['Servers'][$i]['auth_type'] = 'cookie';
$cfg['Servers'][$i]['host'] = '127.0.0.1';
$cfg['Servers'][$i]['port'] = '';
$cfg['Servers'][$i]['compress'] = false;
$cfg['Servers'][$i]['AllowNoPassword'] = true;
$cfg['Servers'][$i]['hide_db'] = '^(information_schema|performance_schema|phpmyadmin|sys|mysql)$';

$cfg['UploadDir'] = '';
$cfg['SaveDir'] = '';

$cfg['PmaAbsoluteUri'] = 'http://127.0.0.1:8080/';
$cfg['ShowServerInfo'] = true;
$cfg['VersionCheck'] = false;
