# curlp

## Overview

curlp is a planned curl-like command-line tool with reusable profiles for
domain-specific authentication. The goal is to configure authentication once for
a domain and reuse it when making HTTP requests.

**Status:** Project concept. This repository has no CLI implementation yet.

## How to Use This

The intended workflow is to define an authentication profile, associate it with a
domain, and make requests using that profile. Supported authentication methods,
configuration format, and command syntax have not been defined yet.

## How to Build Locally

Clone the repository:

```sh
git clone git@github.com:stuft2/curlp.git
cd curlp
```

Build instructions will be added with the implementation.

## How to Publish

No releases or distribution process are available yet.

## Architectural Overview

The planned responsibilities are HTTP request execution, profile configuration,
and selection of authentication settings for the target domain. Implementation
boundaries and credential storage are still to be designed.
