# Security Policy

## Supported Versions

Security fixes target `main` and the latest published minor release line. Use the
latest patch release within that line. Older minor and major release lines do not
receive security backports.

| Version | Supported |
| ------- | --------- |
| `main` | Yes, development fixes |
| Latest published minor release line | Yes, latest patch only |
| Earlier release lines | No |

Until the first tagged release, `main` is the only supported development version.
Development versions may change without compatibility guarantees.

## Reporting a Vulnerability

Report suspected vulnerabilities privately through
[GitHub's Report a vulnerability form](https://github.com/stuft2/httpp/security/advisories/new).
You can also find this under **Security > Advisories > Report a vulnerability**.
Please do not open a public issue for an undisclosed vulnerability.

Include:

- Affected httpp version or commit, operating system, and architecture.
- A description of the vulnerability, its impact, and any prerequisites.
- Reproduction steps or a minimal proof of concept.
- Relevant configuration or logs with tokens, passwords, client secrets, and
  other private information removed.

We aim to acknowledge reports within **7 calendar days** and provide a status
update at least every **14 calendar days** while a report is under active review.
These are response targets for a volunteer-maintained project, not guaranteed
service levels. If an acknowledgment or update is overdue, follow up in the same
private report.

If a report is accepted, we will confirm the affected versions, prioritize a fix
based on impact, and coordinate disclosure with the reporter. Once a fix is
available, we will publish a security advisory with affected and fixed versions,
any available mitigation, and reporter credit if requested. Fix timing depends
on severity and complexity; no fixed resolution deadline is promised.

If a report is declined, we will explain why in the private report and invite
additional evidence or clarification. If it describes a non-security bug, we may
suggest a public issue after confirming that doing so will not disclose sensitive
information.
