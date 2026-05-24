const path = require('path');
const os = require('os');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const TerserPlugin = require('terser-webpack-plugin');

function envFlag(value) {
  return ['1', 'true', 'yes', 'on'].includes(String(value || '').toLowerCase());
}

module.exports = (_env = {}, argv = {}) => {
  const mode = argv.mode || process.env.NODE_ENV || 'development';
  const isProduction = mode === 'production';
  const webTarget = process.env.WHEELMAKER_WEB_TARGET
    ? path.resolve(process.env.WHEELMAKER_WEB_TARGET)
    : path.join(os.homedir(), '.wheelmaker', 'web');

  return {
    mode,
    entry: {
      'runtime-config': path.resolve(__dirname, 'public/runtime-config.js'),
      bundle: path.resolve(__dirname, 'src/main.tsx'),
    },
    output: {
      path: webTarget,
      filename: '[name].js',
      publicPath: '/',
      clean: true,
    },
    resolve: {
      extensions: ['.tsx', '.ts', '.js'],
    },
    module: {
      rules: [
        {
          test: /\.m?js$/,
          resolve: {
            fullySpecified: false,
          },
        },
        {
          test: /\.[jt]sx?$/,
          exclude: /node_modules/,
          use: {
            loader: 'babel-loader',
            options: {
              babelrc: false,
              configFile: false,
              presets: ['@babel/preset-env', '@babel/preset-react', '@babel/preset-typescript'],
            },
          },
        },
        {
          test: /\.css$/,
          use: ['style-loader', 'css-loader'],
        },
        {
          test: /\.(woff2?|ttf|eot|svg)$/,
          type: 'asset/resource',
        },
      ],
    },
    plugins: [
      new HtmlWebpackPlugin({
        template: path.resolve(__dirname, 'public/index.html'),
        inject: false,
      }),
    ],
    devServer: {
      host: '0.0.0.0',
      port: 8080,
      allowedHosts: 'all',
      historyApiFallback: true,
      static: {
        directory: path.resolve(__dirname, 'public'),
      },
    },
    performance: {
      hints: false,
    },
    optimization: {
      minimizer: [new TerserPlugin({ parallel: false })],
    },
    devtool: isProduction && !envFlag(process.env.WHEELMAKER_WEB_SOURCEMAP)
      ? false
      : 'source-map',
  };
};
