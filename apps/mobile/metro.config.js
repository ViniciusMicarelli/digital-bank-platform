// The app imports the shared TypeScript in packages/ts-core (alias
// @bank/core in tsconfig.json), which lives outside the project root.
const { getDefaultConfig } = require('expo/metro-config')
const path = require('path')

const config = getDefaultConfig(__dirname)
config.watchFolders = [path.resolve(__dirname, '../../packages/ts-core')]
config.resolver.nodeModulesPaths = [path.resolve(__dirname, 'node_modules')]

module.exports = config
