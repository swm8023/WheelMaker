import React from 'react';
import {StyleSheet, Text, View} from 'react-native';

function App(): React.JSX.Element {
  return (
    <View style={styles.root}>
      <Text style={styles.text}>WheelMaker uses App.native.tsx for native shell.</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: '#1e1e1e',
  },
  text: {
    color: '#d4d4d4',
  },
});

export default App;
