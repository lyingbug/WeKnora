const lightCodeTheme = require('prism-react-renderer').themes.github;
const darkCodeTheme = require('prism-react-renderer').themes.dracula;

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'WeKnora Docs',
  tagline: 'Open-source knowledge management with RAG, Agent reasoning, and Wiki generation.',
  favicon: 'img/favicon.ico',

  url: 'https://weknora.weixin.qq.com',
  baseUrl: '/docs/',
  organizationName: 'Tencent',
  projectName: 'WeKnora',
  trailingSlash: false,
  markdown: {
    mermaid: true
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en', 'zh-CN'],
    localeConfigs: {
      en: {
        label: 'English',
        direction: 'ltr'
      },
      'zh-CN': {
        label: '简体中文',
        direction: 'ltr'
      }
    }
  },

  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: '/',
          sidebarPath: require.resolve('./sidebars.js'),
          editUrl: 'https://github.com/Tencent/WeKnora/tree/main/docs-site/'
        },
        blog: false,
        theme: {
          customCss: require.resolve('./src/css/custom.css')
        }
      }
    ]
  ],
  themes: ['@docusaurus/theme-mermaid'],

  themeConfig: {
    image: 'img/weknora-social-card.png',
    navbar: {
      title: 'WeKnora',
      logo: {
        alt: 'WeKnora Logo',
        src: 'img/logo.png'
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Docs'
        },
        {
          href: 'https://github.com/Tencent/WeKnora',
          label: 'GitHub',
          position: 'right'
        },
        {
          type: 'localeDropdown',
          position: 'right'
        }
      ]
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {
              label: 'Quick Start',
              to: '/quick-start'
            },
            {
              label: 'Architecture',
              to: '/architecture/overview'
            },
            {
              label: 'API Reference',
              to: '/api/overview'
            }
          ]
        },
        {
          title: 'Community',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/Tencent/WeKnora'
            },
            {
              label: 'Issues',
              href: 'https://github.com/Tencent/WeKnora/issues'
            }
          ]
        }
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Tencent. Built with Docusaurus.`
    },
    prism: {
      theme: lightCodeTheme,
      darkTheme: darkCodeTheme
    }
  }
};

module.exports = config;
