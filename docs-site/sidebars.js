/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docsSidebar: [
    {
      type: 'category',
      label: 'Get Started',
      collapsed: false,
      items: [
        'intro',
        'core-concepts',
        'quick-start',
        'use-cases'
      ]
    },
    {
      type: 'category',
      label: 'User Guide',
      collapsed: false,
      items: [
        'user-guide/knowledge-bases',
        'user-guide/document-ingestion',
        'user-guide/chat-and-rag',
        'user-guide/agent-mode',
        'user-guide/wiki-mode',
        'user-guide/model-configuration',
        'user-guide/mcp-tools'
      ]
    },
    {
      type: 'category',
      label: 'Deployment',
      collapsed: false,
      items: [
        'deployment/docker-compose',
        'deployment/lite-edition',
        'deployment/helm-kubernetes',
        'deployment/environment-variables',
        'deployment/observability'
      ]
    },
    {
      type: 'category',
      label: 'Architecture',
      collapsed: false,
      items: [
        'architecture/overview',
        'architecture/ingestion-pipeline',
        'architecture/retrieval-pipeline',
        'architecture/agent-execution',
        'architecture/wiki-mode',
        'architecture/extension-points'
      ]
    },
    {
      type: 'category',
      label: 'Integrations',
      collapsed: false,
      items: [
        'integrations/data-sources',
        'integrations/im-connectors',
        'integrations/web-search',
        'integrations/vector-stores',
        'integrations/model-providers',
        'integrations/authentication'
      ]
    },
    {
      type: 'category',
      label: 'API Reference',
      collapsed: false,
      items: [
        'api/overview',
        'api/authentication',
        'api/knowledge-base',
        'api/chat',
        'api/agent',
        'api/mcp',
        'api/errors'
      ]
    },
    {
      type: 'category',
      label: 'Developer Guide',
      collapsed: false,
      items: [
        'developer-guide/local-development',
        'developer-guide/project-structure',
        'developer-guide/testing',
        'developer-guide/contributing'
      ]
    },
    {
      type: 'category',
      label: 'Troubleshooting',
      collapsed: false,
      items: [
        'troubleshooting/faq',
        'troubleshooting/logging',
        'troubleshooting/ingestion',
        'troubleshooting/retrieval-quality',
        'troubleshooting/model-calls'
      ]
    }
  ]
};

module.exports = sidebars;
