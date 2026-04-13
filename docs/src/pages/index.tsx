import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <Heading as="h1" className="hero__title">
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle">{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/getting-started">
            Get Started
          </Link>
        </div>
      </div>
    </header>
  );
}

function Features() {
  const features = [
    {
      title: 'Privacy by Design',
      description: 'Prompts and responses are never logged. Audit trails contain only metadata. Enforced in code, not policy.',
    },
    {
      title: 'Open Weights Only',
      description: 'Gemma, Llama, Mistral, Qwen — served locally through Ollama. No closed API proxying, no data leaving your infrastructure.',
    },
    {
      title: 'Community Compute',
      description: 'Volunteers contribute inference capacity by running the gateway-agent alongside Ollama. Ed25519 authenticated, admin-approved.',
    },
  ];

  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {features.map((f, idx) => (
            <div key={idx} className={clsx('col col--4')}>
              <div className="text--center padding-horiz--md padding-vert--lg">
                <Heading as="h3">{f.title}</Heading>
                <p>{f.description}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  return (
    <Layout
      title="Documentation"
      description="Community-governed AI gateway backed by open-weight models">
      <HomepageHeader />
      <main>
        <Features />
      </main>
    </Layout>
  );
}
