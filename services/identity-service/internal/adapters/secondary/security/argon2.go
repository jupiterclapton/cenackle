package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Config par défaut recommandée par OWASP (équilibrée Sécurité/Perf)
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultParams = &Argon2Params{
	Memory:      64 * 1024, // 64 MB
	Iterations:  3,         // Temps de calcul
	Parallelism: 2,         // Threads utilisés
	SaltLength:  16,        // 16 bytes de sel
	KeyLength:   32,        // 32 bytes de hash
}

type Argon2Hasher struct {
	params *Argon2Params
}

func NewArgon2Hasher(params *Argon2Params) *Argon2Hasher {
	if params == nil {
		params = DefaultParams
	}
	return &Argon2Hasher{params: params}
}

// Hash génère le hash Argon2id et retourne une chaîne encodée format PHC.
func (a *Argon2Hasher) Hash(password string) (string, error) {
	// 1. Générer un sel aléatoire
	salt := make([]byte, a.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// 2. Calculer le hash
	hash := argon2.IDKey([]byte(password), salt, a.params.Iterations, a.params.Memory, a.params.Parallelism, a.params.KeyLength)

	// 3. Encoder en base64 (RawStdEncoding évite le padding '=')
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	// 4. Formater la chaîne standard
	// Format: $argon2id$v=19$m=65536,t=3,p=2$salt$hash
	encodedHash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, a.params.Memory, a.params.Iterations, a.params.Parallelism, b64Salt, b64Hash)

	return encodedHash, nil
}

// Compare vérifie le mot de passe hashé.
func (a *Argon2Hasher) Compare(encodedHash, password string) error {
	// 1. Parser la chaîne encodée pour récupérer le sel et les paramètres utilisés À L'ÉPOQUE
	p, salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return err
	}

	// 2. Calculer le hash du mot de passe candidat avec les MÊMES paramètres
	otherHash := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

	// 3. Comparaison à temps constant (Crucial pour la sécurité)
	if subtle.ConstantTimeCompare(hash, otherHash) == 1 {
		return nil
	}
	return errors.New("invalid password")
}

// --- Helpers de décodage ---

func decodeHash(encodedHash string) (p *Argon2Params, salt, hash []byte, err error) {
	vals := strings.Split(encodedHash, "$")
	if len(vals) != 6 {
		return nil, nil, nil, errors.New("invalid hash format")
	}

	var version int
	_, err = fmt.Sscanf(vals[2], "v=%d", &version)
	if err != nil {
		return nil, nil, nil, err
	}
	if version != argon2.Version {
		return nil, nil, nil, errors.New("incompatible argon2 version")
	}

	p = &Argon2Params{}
	_, err = fmt.Sscanf(vals[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism)
	if err != nil {
		return nil, nil, nil, err
	}

	salt, err = base64.RawStdEncoding.DecodeString(vals[4])
	if err != nil {
		return nil, nil, nil, err
	}
	p.SaltLength = uint32(len(salt))

	hash, err = base64.RawStdEncoding.DecodeString(vals[5])
	if err != nil {
		return nil, nil, nil, err
	}
	p.KeyLength = uint32(len(hash))

	return p, salt, hash, nil
}
