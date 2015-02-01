BlobPad
=======

BlobPad is a note taking application build on top of [BlobStash](https://github.com/tsileo/blobstash).

## Requirements

- [BlobStash](https://github.com/tsileo/blobstash)
- [Elasticsearch](http://www.elasticsearch.org/)
- **pdftotext** from [Xpdf](http://www.foolabs.com/xpdf/).

## Features

- Notes can be Markdown, or uploaded PDF
- Full notes history
- Full text search (pdf are also indexed)
- Possibility to encrypt notes using [NaCl secretbox](http://nacl.cr.yp.to/secretbox.html) (via BlobStash)
- No delete feature by design (a virtual trash may be implemented)

## Installation

Assuming you have installed/configured [BlobStash](https://github.com/tsileo/blobstash) and [Elasticsearch](http://www.elasticsearch.org/).

If you want PDF to be parsed for indexing, you need to install **pdftotext** from [Xpdf](http://www.foolabs.com/xpdf/).

## Usage

Just run ``blobpad`` and open **localhost:8000**.

```console
$ blobpad
```

### Re-indexing

You may need to re-index notes into elasticsearch, to do so, just run:

```console
$ curl http://localhost:8000/_reindex
```

## Based upon

- [BlobStash](https://github.com/tsileo/blobstash), for blob store/database
- [UIkit](http://getuikit.com/), [Ractive.js](http://www.ractivejs.org/), [CodeMirror](http://codemirror.net/), for the UI
- [Elasticsearch](http://www.elasticsearch.org/), for the search
- pdftotext and [pdf.js](https://github.com/mozilla/pdf.js) for parsing/viewing PDF

## Roadmap

- A virtual trash
- Hawk-based authentication
- JPEG support (+ OCR)
- Bookmarklet to save website with selected plain-text content
- An Evernote-like Android app
- Create new note by email?
- Ability to share note via [BlobShare](https://github.com/tsileo/blobshare) ?

## Donate

[![Flattr this git repo](http://api.flattr.com/button/flattr-badge-large.png)](https://flattr.com/submit/auto?user_id=tsileo&url=https%3A%2F%2Fgithub.com%2Ftsileo%2Fblobpad)

BTC 1JV2PCgBNRM7bQ2uKB5F4Nd6bUroyzQJ6T

## License

Copyright (c) 2014 Thomas Sileo and contributors. Released under the MIT license.
