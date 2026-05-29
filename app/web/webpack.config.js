const path = require('path');
const os = require('os');
const { execFileSync } = require('child_process');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const TerserPlugin = require('terser-webpack-plugin');
const webpack = require('webpack');

function envFlag(value) {
  return ['1', 'true', 'yes', 'on'].includes(String(value || '').toLowerCase());
}

function gitValue(args) {
  try {
    return execFileSync('git', args, {
      cwd: path.resolve(__dirname, '..'),
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
  } catch {
    return '';
  }
}

function jsFilename(isProduction) {
  return pathData => {
    if (isProduction && pathData?.chunk?.name === 'bundle') {
      return 'bundle.[contenthash].js';
    }
    return '[name].js';
  };
}

function jsChunkFilename(isProduction) {
  return () => (isProduction ? '[name].[contenthash].js' : '[name].js');
}

function cssFilename(isProduction) {
  return pathData => {
    if (isProduction && pathData?.chunk?.name === 'bundle') {
      return 'bundle.[contenthash].css';
    }
    return '[name].css';
  };
}

function cssChunkFilename(isProduction) {
  return () => (isProduction ? '[name].[contenthash].css' : '[name].css');
}

module.exports = (_env = {}, argv = {}) => {
  const mode = argv.mode || process.env.NODE_ENV || 'development';
  const isProduction = mode === 'production';
  const webTarget = process.env.WHEELMAKER_WEB_TARGET
    ? path.resolve(process.env.WHEELMAKER_WEB_TARGET)
    : path.join(os.homedir(), '.wheelmaker', 'web');
  const webBuildSha = process.env.WHEELMAKER_WEB_BUILD_SHA || gitValue(['rev-parse', 'HEAD']);
  const webBuildTime = process.env.WHEELMAKER_WEB_BUILD_TIME || new Date().toISOString();

  return {
    mode,
    entry: {
      'runtime-config': path.resolve(__dirname, 'public/runtime-config.js'),
      bundle: path.resolve(__dirname, 'src/main.tsx'),
    },
    output: {
      path: webTarget,
      filename: jsFilename(isProduction),
      chunkFilename: jsChunkFilename(isProduction),
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
          use: [isProduction ? MiniCssExtractPlugin.loader : 'style-loader', 'css-loader'],
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
      new webpack.DefinePlugin({
        __WHEELMAKER_WEB_BUILD_SHA__: JSON.stringify(webBuildSha),
        __WHEELMAKER_WEB_BUILD_TIME__: JSON.stringify(webBuildTime),
      }),
      ...(isProduction ? [new MiniCssExtractPlugin({
        filename: cssFilename(isProduction),
        chunkFilename: cssChunkFilename(isProduction),
      })] : []),
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
