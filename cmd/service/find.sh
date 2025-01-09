find / -type f \
-exec md5 {} \; 2>/dev/null | awk -F "MD5 .|\) = " '{print $3 " " $2}' > find.txt
