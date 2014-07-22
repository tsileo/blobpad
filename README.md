BlobPad
=======

BlobPad is a note taking application build on top of [BlobStash](https://github.com/tsileo/blobstash).

## Features

- Notes can be Markdown, or uploaded PDF
- Full notes history
- Full text search (pdf are also indexed)
- Possibility to encrypt notes using [NaCl secretbox](http://nacl.cr.yp.to/secretbox.html) (via BlobStash)
- No delete feature by design, deleted notes/notebooks stay in a special trash

## Based upon

- [BlobStash](https://github.com/tsileo/blobstash), for blob store/database
- [UIkit](http://getuikit.com/), [Ractive.js](http://www.ractivejs.org/), [CodeMirror](http://codemirror.net/), for the UI
- [Elasticsearch](http://www.elasticsearch.org/), for the search
- pdftotext and [pdf.js](https://github.com/mozilla/pdf.js) for parsing/viewing PDF

## Roadmap

- JPEG support (+ OCR)
- Bookmarklet to save website with selected plain-text content
- An Evernote-like Android app
