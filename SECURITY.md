# Security policy

## What the guards are

The guards under `setup/guards/` are a **tripwire, not a sandbox**. They stop a
brainer from writing into your repositories by accident or by an agent's honest
mistake, and they say so loudly when it tries. They do not contain an agent, or
a person, who is set on getting around them on a machine they already control.
A bypass of that kind is a vector to close (open a "Guard vector" issue), not a
vulnerability.

## Reporting a vulnerability

If you find something that harms a user who did nothing wrong — a command that
leaks a secret, writes outside the places it declares, or runs attacker-chosen
code from a repository, brief or message the user only read — report it
privately: on this repository's **Security** tab, choose **Report a
vulnerability**. Please do not open a public issue for it.

Include the command or input, the platform, the output of
`./install.sh --version`, and what you expected. This is a small project run
by one maintainer: expect an acknowledgement within a week, not an SLA.

## Supported versions

The latest release candidate and `main`. There are no backports.
