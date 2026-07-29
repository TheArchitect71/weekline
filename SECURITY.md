# Security policy

Weekline is preparing for its first production deployment. Please do not open
public issues containing credentials, employee information, host-control
tokens, database URLs, or other sensitive operational details.

Report a suspected vulnerability privately through the repository owner's
GitHub profile until a dedicated security contact is published. Include the
affected version, reproduction steps, and likely impact without including real
employee data.

Only the latest tagged release is supported. Production operators must keep
Windows, PostgreSQL, Caddy, and Weekline patched; expose only TCP ports 80 and
443; and keep database, API, Caddy administration, and Remote Desktop ports
closed to the public Internet.
