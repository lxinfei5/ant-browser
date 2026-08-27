package forcefont

import "embed"

// ExtensionID is the Chrome ID derived from ManifestKey (SHA-256 of the SPKI, 0-f → a-p).
const ExtensionID = "kibjgedglpgomepboimjhcalfabmjakl"

// ManifestKey is the base64 SPKI placed in manifest.json "key" so Chrome keeps a stable ID.
const ManifestKey = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0hO4LIBLrkFJeCcs2/qfT8ncMEznPH03DvEYZYTU1+I89fWtM6DbM3mN/4ZTlJgJCogvXCf5mBkeZKPuD0cZbzHS8RdG4JL1iOs4SmaL3gxox/4cBTVthXDREev4EnOIDEkwiIWd/ZS7ohL32MytC9UZTjL39rOF1tM7lx5Xu2XthQy4Qthp240aFQN274BaV/hCtqr2sC+HCqe/alMp5EWgE6zfubatXgb5qwL8RfQRQEi8YXCX2ciuMVc/TXj65gbhM67hOlLg/jtDKd5zuaHgppnr5V/yI6zRTJ/JaCLsJU7q8Nxz7pltCx30noI8hIMNPHpw1h0v6wGLMDNq+wIDAQAB"

const SourceURL = "builtin://force-font"

const Version = "4.0.0"

//go:embed all:files
var Files embed.FS
