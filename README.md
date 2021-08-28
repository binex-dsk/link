Fork of [link](https://fsh.ee/) with extra features, a filesystem-based backend, and more.

Please access this project on my [Gitea](https://git.swurl.xyz/swirl/link) instance, NOT GitHub.

# Self-Hosting
You can host this yourself.

Note: all commands here are done as root.

## Building & Installing
To build this project, you'll need [Go](https://golang.org/doc/install) and [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git). Most Linux distributions should have these in their repositories, i.e.:
- `pacman -S go git`
- `emerge --ask dev-lang/go dev-vcs/git`
- `apt install go git`

1. Clone this repository:

```bash
git clone https://git.swurl.xyz/swirl/link && cd link
```

2. Compile:
```bash
make
```

3. Now, you need to install. NGINX and systemd files are provided in this project; you may choose not to install them.

For all install commands, you may optionally provide `prefix` and `DESTDIR` options. This is useful for packagers; i.e. for a PKGBUILD: `make prefix=/usr DESTDIR=${pkgdir} install`.

Available install commands are as follows:
- `make install` installs the executable, NGINX, and systemd files.
- `make install-bin` installs the executable file.
- `make install-systemd` installs the systemd file, as well as its environment file.
- `make install-nginx` installs the NGINX file.

For example, on a non-systemd system using NGINX, you would run `make install-bin install-nginx`.

4. If using systemd, change the environment file to reflect your desired options:
```bash
vim /etc/link.conf
```

5. You can now enable and start the service:
```bash
systemctl enable --now link
```

The server should now be running on localhost at port 8080.

## NGINX Reverse Proxy
An NGINX file is provided with this project. Sorry, no support for Apache or lighttpd or anything else; should've chosen a better HTTP server.

For this, you'll need [NGINX](https://nginx.org/en/download.html) (obviously), certbot, and its NGINX plugin. Most Linux distributions should have these in their repositories, i.e.:
- `pacman -S nginx certbot-nginx`
- `emerge --ask www-servers/nginx app-crypt/certbot-nginx`
- `apt install nginx python-certbot-nginx`

This section assumes you've already followed the last.

1. Change the domain in the NGINX file:
```bash
sed -i 's/your.doma.in/[DOMAIN HERE]' /etc/nginx/sites-available/link
```

2. Enable the site:
```bash
ln -s /etc/nginx/sites-{available,enabled}/link
```

3. Enable HTTPS for the site:
```bash
certbot --nginx -d [DOMAIN HERE]
```

4. Enable and start NGINX:
```bash
systemctl enable --now nginx
```

If it's already running, reload:
```bash
systemctl reload nginx
```

Your site should be running at https://your.doma.in. Test it by going there, and trying the examples. If they don't work, open an issue.

# Contributions
Contributions are always welcome.

# FAQ
## A user has made a link to a bad site! What do I do?
Clean it up, janny!

Deleting a link can be done simply by running:
```bash
rm /srv/link/*/BADLINKHERE
```

Replace `/srv/link` with whatever your data directory is.

## Can I prevent users from making links to specific sites (i.e. illegal content)?
Not currently. Might implement this in the future.

## Can I run this in a subdirectory of my site?
Yes. Simply put the `proxy_pass` directive in a subdirectory, i.e.:
```
location /shortener {
    proxy_pass http://localhost:8080;
}
```

## Why'd you make this fork?
While link was by far the best link shortener I could find, it had a few problems:
- No query-string support: had to make POST requests
- Didn't decode URLs
- SQLite is not the greatest storage method out there
- No pre-provided systemd or NGINX files
- No `install` target for the makefile

The first two are mostly problems when using them with specific services; i.e. PrivateBin, which expects to be able to use query-strings and encoded URLs.
