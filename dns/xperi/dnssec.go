// Copyright 2024 TochusC AOSP Lab. All rights reserved.

// dnssec.go 提供了一些DNSSEC相关的测试用函数。

package xperi

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/big"

	"github.com/tochusc/godns/dns"
)

// ParseKeyBase64 解析 Base64 编码的密钥为字节切片
func ParseKeyBase64(keyb64 string) []byte {
	keyBytes, err := base64.StdEncoding.DecodeString(keyb64)
	if err != nil {
		panic(fmt.Sprintf("failed to decode base64 key: %s", err))
	}
	return keyBytes
}

// CalculateKeyTag 计算 DNSKEY 的 Key Tag
//   - 传入 DNSKEY RDATA
//   - 返回 Key Tag
//
// Key Tag 是 DNSKEY 的一个 16 位无符号整数，用于快速识别 DNSKEY
func CalculateKeyTag(key dns.DNSRDATADNSKEY) uint16 {
	rdata := key.Encode()
	var ac uint32
	for i := 0; i < len(rdata); i++ {
		if i&1 == 1 {
			ac += uint32(rdata[i])
		} else {
			ac += uint32(rdata[i]) << 8
		}
	}
	ac += ac >> 16 & 0xFFFF
	return uint16(ac & 0xFFFF)
}

// GenerateDNSKEY 生成公钥的 DNSKEY RDATA, 并返回私钥字节
// 传入参数：
//   - algo: DNSSEC 算法
//   - flag: DNSKEY Flag
//
// 返回值：
//   - 公钥 DNSKEY RDATA
//   - 私钥字节
func GenerateDNSKEY(algo DNSSECAlgorithm, flag DNSKEYFlag) (dns.DNSRDATADNSKEY, []byte) {
	algorithmer := DNSSECAlgorithmerFactory(algo)
	privKey, pubKey := algorithmer.GenerateKey()
	return dns.DNSRDATADNSKEY{
		Flags:     flag,
		Protocol:  3,
		Algorithm: algo,
		PublicKey: pubKey,
	}, privKey
}

// GenerateRRSIG 生成 RRSIG RDATA，
// 该函数目前无法将传入的 RRSET 进行 规范化 及 规范化排序，
// 所以需要外部保证传入的 RRSET 是规范的，才可以成功生成正确的 RRSIG。
// 传入参数：
//   - rrSet: 要签名的 RR 集合
//   - algo: 签名算法
//   - expiration: 签名过期时间
//   - inception: 签名生效时间
//   - keyTag: 签名公钥的 Key Tag
//   - signerName: 签名者名称
//   - privKey: 签名私钥的 字节编码
//
// 返回值：
//   - RRSIG RDATA
//
// signature = sign(RRSIG_RDATA | RR(1) | RR(2) | ...)
func GenerateRRSIG(rrSet []DNSResourceRecord, algo DNSSECAlgorithm,
	expiration, inception uint32, keyTag uint16,
	signerName string, privKey []byte) DNSRDATARRSIG {

	// signature = sign(RRSIG_RDATA | RR(1) | RR(2) | ...)
	// RRSIG_RDATA
	rrsig := DNSRDATARRSIG{
		TypeCovered: rrSet[0].Type,
		Algorithm:   algo,
		Labels:      uint8(CountDomainNameLabels(&rrSet[0].Name)),
		OriginalTTL: rrSet[0].TTL,
		Expiration:  expiration,
		Inception:   inception,
		KeyTag:      uint16(keyTag),
		SignerName:  signerName,
		Signature:   []byte{},
	}

	plainLen := rrsig.Size()
	for _, rr := range rrSet {
		plainLen += rr.Size()
	}
	plainText := make([]byte, plainLen)
	offset, err := rrsig.EncodeToBuffer(plainText)
	if err != nil {
		panic(fmt.Sprintf("failed to encode RRSIG RDATA: %s", err))
	}
	// TODO: 规范化RRSET，Canonicalize the RRs
	// 现在只能依赖于外部保证传入的 RRSET 是规范的
	// RR = owner | type | class | TTL | RDATA length | RDATA
	for _, rr := range rrSet {
		increment, err := rr.EncodeToBuffer(plainText[offset:])
		if err != nil {
			panic(fmt.Sprintf("failed to encode RR: %s", err))
		}
		offset += increment
	}

	if offset != plainLen {
		panic("failed to encode RRSIG RDATA: unexpected offset")
	}

	// 接口以及工厂模式 Coooool
	var signature []byte
	algorithmer := DNSSECAlgorithmerFactory(algo)
	signature, err = algorithmer.Sign(plainText, privKey)
	if err != nil {
		panic(fmt.Sprintf("failed to sign RRSIG: %s", err))
	}

	// 之前的旧有实现
	// switch algo {
	// case DNSSECAlgorithmRSASHA1:
	// 	signature, err = RSASHA1Sign(plainText, privKey)
	// case DNSSECAlgorithmRSASHA256:
	// 	signature, err = RSASHA256Sign(plainText, privKey)
	// case DNSSECAlgorithmRSASHA512:
	// 	signature, err = RSASHA512Sign(plainText, privKey)
	// case DNSSECAlgorithmECDSAP256SHA256:
	// 	signature, err = ECDSAP256SHA256Sign(plainText, privKey)
	// case DNSSECAlgorithmECDSAP384SHA384:
	// 	signature, err = ECDSAP384SHA384Sign(plainText, privKey)
	// default:
	// 	panic(fmt.Sprintf("unsupported algorithm: %d", algo))
	// }
	// if err != nil {
	// 	panic(fmt.Sprintf("failed to sign RRSIG: %s", err))
	// }

	rrsig.Signature = signature

	return rrsig
}

// GenerateDS 生成DNSKEY的 DS RDATA
// 传入参数：
//   - oName: DNSKEY 的所有者名称
//   - kRDATA: DNSKEY RDATA
//   - dType: 所使用的摘要算法类型
//
// 返回值：
//   - DS RDATA
//
// digest = digest_algorithm( DNSKEY owner name | DNSKEY RDATA);
func GenerateDS(oName string, kRDATA dns.DNSRDATADNSKEY, dType dns.DNSSECDigestType) dns.DNSRDATADS {
	// 1. 计算 DNSKEY 的 Key Tag
	keyTag := CalculateKeyTag(kRDATA)

	// 2. 构建明文
	pText := make([]byte, dns.GetDomainNameWireLen(&oName)+kRDATA.Size())
	offset, err := dns.EncodeDomainNameToBuffer(&oName, pText)
	if err != nil {
		panic(fmt.Sprintf("failed to write domain name: %s", err))
	}
	_, err = kRDATA.EncodeToBuffer(pText[offset:])
	if err != nil {
		panic(fmt.Sprintf("failed to encode DNSKEY RDATA: %s", err))
	}

	var digest []byte
	// 3. 计算摘要
	switch dType {
	case dns.DNSSECDigestTypeSHA1:
		nDigest := sha1.Sum(pText)
		digest = nDigest[:]
	case dns.DNSSECDigestTypeSHA256:
		nDigest := sha256.Sum256(pText)
		digest = nDigest[:]
	case dns.DNSSECDigestTypeSHA384:
		nDigest := sha512.Sum384(pText)
		digest = nDigest[:]
	default:
		panic(fmt.Sprintf("unsupported digest type: %d", dType))
	}

	// 4. 构建 DS RDATA
	return DNSRDATADS{
		KeyTag:     keyTag,
		Algorithm:  kRDATA.Algorithm,
		DigestType: dType,
		Digest:     digest[:],
	}
}

// DNSSECAlgorithmer DNSSEC 算法接口
type DNSSECAlgorithmer interface {
	// Sign 使用私钥对数据进行签名
	Sign(data, privKey []byte) ([]byte, error)
	// GenerateKey 生成密钥对
	GenerateKey() ([]byte, []byte)
}

// DNSSECAlgorithmFactory 生成 DNSSECAlgorithmer
func DNSSECAlgorithmerFactory(algo DNSSECAlgorithm) DNSSECAlgorithmer {
	switch algo {
	case DNSSECAlgorithmRSASHA1:
		return RSASHA1{}
	case DNSSECAlgorithmRSASHA256:
		return RSASHA256{}
	case DNSSECAlgorithmRSASHA512:
		return RSASHA512{}
	case DNSSECAlgorithmECDSAP256SHA256:
		return ECDSAP256SHA256{}
	case DNSSECAlgorithmECDSAP384SHA384:
		return ECDSAP384SHA384{}
	default:
		panic(fmt.Sprintf("unsupported algorithm: %d", algo))
	}
}

type RSASHA1 struct{}

func (RSASHA1) Sign(data, privKey []byte) ([]byte, error) {
	// 计算明文摘要
	digest := sha1.Sum(data)

	// 重建 RSA 私钥
	pKey, err := x509.ParsePKCS1PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %s", err)
	}

	// 签名
	signature, err := rsa.SignPKCS1v15(nil, pKey, crypto.SHA256, digest[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %s", err)
	}

	return signature, nil
}

func (RSASHA1) GenerateKey() ([]byte, []byte) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate RSA key: %s", err))
	}

	privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal public key: %s", err))
	}

	return privKeyBytes, pubKeyBytes
}

type RSASHA256 struct{}

func (RSASHA256) Sign(data, privKey []byte) ([]byte, error) {
	// 计算明文摘要
	digest := sha256.Sum256(data)

	// 重建 RSA 私钥
	pKey, err := x509.ParsePKCS1PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %s", err)
	}

	// 签名
	signature, err := rsa.SignPKCS1v15(nil, pKey, crypto.SHA256, digest[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %s", err)
	}

	return signature, nil
}

func (RSASHA256) GenerateKey() ([]byte, []byte) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate RSA key: %s", err))
	}

	privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal public key: %s", err))
	}

	return privKeyBytes, pubKeyBytes
}

type RSASHA512 struct{}

func (RSASHA512) Sign(data, privKey []byte) ([]byte, error) {
	// 计算明文摘要
	digest := sha512.Sum512(data)

	// 重建 RSA 私钥
	pKey, err := x509.ParsePKCS1PrivateKey(privKey)

	// 签名
	signature, err := rsa.SignPKCS1v15(nil, pKey, crypto.SHA512, digest[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %s", err)
	}

	return signature, nil
}

func (RSASHA512) GenerateKey() ([]byte, []byte) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate RSA key: %s", err))
	}

	privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal public key: %s", err))
	}

	return privKeyBytes, pubKeyBytes
}

type ECDSAP256SHA256 struct{}

func (ECDSAP256SHA256) Sign(data, privKey []byte) ([]byte, error) {
	// 计算明文摘要
	digest := sha256.Sum256(data)

	// 重建 ECDSA 私钥
	curve := elliptic.P256()
	pKey := new(ecdsa.PrivateKey)
	pKey.PublicKey.Curve = curve
	pKey.D = new(big.Int).SetBytes(privKey)
	pKey.PublicKey.X, pKey.PublicKey.Y = curve.ScalarBaseMult(privKey)

	// 签名
	r, s, err := ecdsa.Sign(rand.Reader, pKey, digest[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %s", err)
	}

	signature := append(r.Bytes(), s.Bytes()...)

	return signature, nil
}

func (ECDSAP256SHA256) GenerateKey() ([]byte, []byte) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("failed to generate ECDSA key: %s", err))
	}
	privKeyBytes := privKey.D.Bytes()
	pubKeyBytes := append(privKey.PublicKey.X.Bytes(), privKey.PublicKey.Y.Bytes()...)
	return privKeyBytes, pubKeyBytes
}

type ECDSAP384SHA384 struct{}

func (ECDSAP384SHA384) Sign(data, privKey []byte) ([]byte, error) {
	// 计算明文摘要
	digest := sha512.Sum384(data)

	// 重建 ECDSA 私钥
	curve := elliptic.P384()
	pKey := new(ecdsa.PrivateKey)
	pKey.PublicKey.Curve = curve
	pKey.D = new(big.Int).SetBytes(privKey)
	pKey.PublicKey.X, pKey.PublicKey.Y = curve.ScalarBaseMult(privKey)

	// 签名
	r, s, err := ecdsa.Sign(rand.Reader, pKey, digest[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %s", err)
	}

	signature := append(r.Bytes(), s.Bytes()...)

	return signature, nil
}

func (ECDSAP384SHA384) GenerateKey() ([]byte, []byte) {
	privKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("failed to generate ECDSA key: %s", err))
	}
	privKeyBytes := privKey.D.Bytes()
	pubKeyBytes := append(privKey.PublicKey.X.Bytes(), privKey.PublicKey.Y.Bytes()...)
	return privKeyBytes, pubKeyBytes
}

// GenWrongKey 生成一个具有指定KeyTag，且能通过检验，但错误的 DNSKEY RDATA
// 传入参数：
//   - algo: DNSSEC 算法
//   - flag: DNSKEY Flag
//   - tag: Key Tag
//
// 返回值：
//   - 你想要的 DNSKEY RDATA
func GenWrongKeyWithTag(algo dns.DNSSECAlgorithm, flag dns.DNSKEYFlag, tag int) dns.DNSRDATADNSKEY {
	algorithmer := DNSSECAlgorithmerFactory(algo)
	_, pubKey := algorithmer.GenerateKey()
	pKey := dns.DNSRDATADNSKEY{
		Flags:     flag,
		Protocol:  3,
		Algorithm: algo,
		PublicKey: pubKey,
	}

	rTag := CalculateKeyTag(pKey)
	dif := tag - int(rTag)

	fmt.Printf("rTag:%d, tTag:%d, dif: %d\n", rTag, tag, dif)

	if dif < 0 {
		dif = -dif
		hDif := dif >> 8
		lDif := dif & 0xFF

		for tvlr, _ := range pubKey {
			if tvlr&1 == 0 {
				if int(pubKey[tvlr])-int(hDif) < 0 {
					pubKey[tvlr] = 0
					hDif -= int(pubKey[tvlr])
				} else {
					pubKey[tvlr] -= byte(hDif)
					hDif = 0
				}
			} else {
				if int(pubKey[tvlr])-int(hDif) < 0 {
					pubKey[tvlr] = 0
					lDif -= int(pubKey[tvlr])
				} else {
					pubKey[tvlr] -= byte(lDif)
					lDif = 0
				}
			}
			if hDif == 0 && lDif == 0 {
				break
			}
		}
	} else {
		hDif := dif >> 8
		lDif := dif & 0xFF

		for tvlr, _ := range pubKey {
			if tvlr&1 == 0 {
				if int(pubKey[tvlr])+int(hDif) > 0xFF {
					pubKey[tvlr] = 0xFF
					hDif -= int(0xFF) - int(pubKey[tvlr])
				} else {
					pubKey[tvlr] += byte(hDif)
					hDif = 0
				}
			} else {
				if int(pubKey[tvlr])+int(lDif) > 0xFF {
					pubKey[tvlr] = 0xFF
					lDif -= int(0xFF) - int(pubKey[tvlr])
				} else {
					pubKey[tvlr] += byte(lDif)
					lDif = 0
				}
			}
			if hDif == 0 && lDif == 0 {
				break
			}
		}
	}

	// 重新计算 Key Tag, 算法不能保证成功
	if rTag != uint16(tag) {
		return GenWrongKeyWithTag(algo, flag, tag)
	}

	return pKey
}

// GenKeyWithTag 生成一个具有指定KeyTag的 DNSKEY RDATA
// 传入参数：
//   - algo: DNSSEC 算法
//   - flag: DNSKEY Flag
//   - tag: Key Tag
//
// 返回值：
//   - 你想要的 DNSKEY RDATA
//
// 注意：这个函数会十分耗时，因为它会尝试生成大量的密钥对，直到找到一个符合要求的密钥对。
func GenKeyWithTag(algo dns.DNSSECAlgorithm, flag dns.DNSKEYFlag, tag int) dns.DNSRDATADNSKEY {
	for {
		algorithmer := DNSSECAlgorithmerFactory(algo)
		_, pubKey := algorithmer.GenerateKey()
		pKey := dns.DNSRDATADNSKEY{
			Flags:     flag,
			Protocol:  3,
			Algorithm: algo,
			PublicKey: pubKey,
		}

		rTag := dns.CalculateKeyTag(pKey)
		if int(rTag) == tag {
			return pKey
		}
	}
}

// GenRandomRRSIG 生成一个随机(同时也会是错误的)的 RRSIG RDATA
// 传入参数：
//   - rrSet: 要签名的 RR 集合
//   - algo: 签名算法
//   - expiration: 签名过期时间
//   - inception: 签名生效时间
//   - keyTag: 签名公钥的 Key Tag
//   - signerName: 签名者名称
//
// 返回值：
//   - 你想要的 RRSIG RDATA
func GenRandomRRSIG(rrSet []dns.DNSResourceRecord, algo dns.DNSSECAlgorithm,
	expiration, inception uint32, keyTag uint16, signerName string) dns.DNSRDATARRSIG {

	algorithmer := DNSSECAlgorithmerFactory(algo)
	privKey, _ := algorithmer.GenerateKey()
	rText := []byte("random plaintext")
	sig, err := algorithmer.Sign(rText, privKey)
	if err != nil {
		panic(fmt.Sprintf("function GenRandomRRSIG() failed:\n%s", err))
	}

	_, err = rand.Read(sig)
	if err != nil {
		panic(fmt.Sprintf("function GenRandomRRSIG() failed:\n%s", err))
	}

	return dns.DNSRDATARRSIG{
		TypeCovered: rrSet[0].Type,
		Algorithm:   algo,
		Labels:      uint8(dns.CountDomainNameLabels(&rrSet[0].Name)),
		OriginalTTL: 3600,
		Expiration:  expiration,
		Inception:   inception,
		KeyTag:      keyTag,
		SignerName:  signerName,
		Signature:   sig,
	}
}
