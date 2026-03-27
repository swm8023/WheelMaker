import AsyncStorage from '@react-native-async-storage/async-storage';

const KEY_LAST_REGISTRY_ADDRESS = 'wheelmaker.lastRegistryAddress';

export async function loadLastRegistryAddress(): Promise<string> {
  try {
    const value = await AsyncStorage.getItem(KEY_LAST_REGISTRY_ADDRESS);
    return value?.trim() ?? '';
  } catch {
    return '';
  }
}

export async function saveLastRegistryAddress(address: string): Promise<void> {
  try {
    await AsyncStorage.setItem(KEY_LAST_REGISTRY_ADDRESS, address.trim());
  } catch {
    // Ignore persistence failures and keep connect flow unaffected.
  }
}
