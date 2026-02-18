server {
    listen 443 ssl http2;
    server_name __USERNAME__.claw.gather.is;

    ssl_certificate /etc/letsencrypt/live/claw.gather.is/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/claw.gather.is/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:__PORT__;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
        proxy_send_timeout 86400;
    }
}
