# WeKnora Docs Site

This directory contains the source for the public WeKnora documentation site.

## Local Development

```bash
cd docs-site
npm install
npm run start
```

Chinese preview:

```bash
npm run start:zh
```

## Production Build

```bash
npm run build
```

The site is configured with `baseUrl: '/docs/'`, so the generated output can be mounted under:

```text
https://weknora.weixin.qq.com/docs/
```

The Chinese locale is generated under:

```text
https://weknora.weixin.qq.com/docs/zh-CN/
```

## Content Rules

- English is the default locale.
- Chinese pages live under `i18n/zh-CN/docusaurus-plugin-content-docs/current/`.
- Keep slugs in English for stable external links.
- Translate titles and body text, but keep the directory structure aligned between locales.
- Treat the old repository-level `docs/` directory as source material, not as the information architecture.
