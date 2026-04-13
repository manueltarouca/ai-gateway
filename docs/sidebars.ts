import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'getting-started',
    'architecture',
    {
      type: 'category',
      label: 'Gateway',
      items: [
        'gateway/configuration',
        'gateway/virtual-keys',
        'gateway/audit-logging',
      ],
    },
    {
      type: 'category',
      label: 'Community Tier',
      items: [
        'community/overview',
        'community/gateway-agent',
        'community/admin-api',
      ],
    },
  ],
};

export default sidebars;
