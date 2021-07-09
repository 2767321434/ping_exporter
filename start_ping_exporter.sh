chmod 755 ./main
nohup sudo ./main -port 8889 -pingaddr www.baidu.com -count 4 1>/dev/null 2>&1 &
