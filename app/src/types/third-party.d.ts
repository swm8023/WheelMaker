declare module 'prismjs' {
  type PrismToken = string | {type: string; alias?: string | string[]; content: PrismToken | PrismToken[]};
  type PrismGrammar = Record<string, unknown>;

  const Prism: {
    languages: Record<string, PrismGrammar>;
    tokenize: (text: string, grammar: PrismGrammar) => PrismToken[];
  };

  export = Prism;
}
