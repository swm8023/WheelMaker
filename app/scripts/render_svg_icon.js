const fs = require('fs');
const path = require('path');
const { Resvg } = require('@resvg/resvg-js');

const [, , sourceArg, targetArg, sizeArg] = process.argv;

if (!sourceArg || !targetArg) {
  console.error('usage: node scripts/render_svg_icon.js <source.svg> <target.png> [size]');
  process.exit(2);
}

const size = Number.isFinite(Number(sizeArg)) ? Math.max(16, Math.trunc(Number(sizeArg))) : 1024;
const source = path.resolve(sourceArg);
const target = path.resolve(targetArg);
const svg = fs.readFileSync(source);
const rendered = new Resvg(svg, {
  fitTo: {
    mode: 'width',
    value: size,
  },
}).render();

fs.mkdirSync(path.dirname(target), { recursive: true });
fs.writeFileSync(target, rendered.asPng());
