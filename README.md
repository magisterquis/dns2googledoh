DNS2GoogleDoH
=============
Serves DNS and proxies queries to Google's DNS-over-HTTP service,
domain-fronted to a configurable domain.

For legal use only.

Usage
-----
Set it running somewhere and point your box's default resolver to it (e.g. via
`/etc/resolv.conf`).  If you run it locally and set your resolver to
`localhost`, it'll probably be necessary to put the IP address of the SNI
domain in `/etc/hosts`.

Front Domain
------------
By default, the front domain is `youtube.com` though several other Google
domains work as well, to include `google.com`.
