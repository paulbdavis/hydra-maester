package controllers_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/utils/pointer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	hydrav1alpha1 "github.com/ory/hydra-maester/api/v1alpha1"
	"github.com/ory/hydra-maester/controllers"
	"github.com/ory/hydra-maester/controllers/mocks"
	"github.com/ory/hydra-maester/hydra"
	. "github.com/stretchr/testify/mock"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	timeout      = time.Second * 5
	tstNamespace = "default"
	tstSecret    = "testSecret"
)

var clientIter = 0

func makeClientInfo() (string, string, string) {
	defer func() { clientIter++ }()
	return fmt.Sprintf("test-client-%d", clientIter),
		fmt.Sprintf("test-client-id-%d", clientIter),
		fmt.Sprintf("test-client-secret-%d", clientIter)
}

var _ = Describe("OAuth2Client Controller", func() {

	Context("in a happy-path scenario", func() {

		Context("should call create OAuth2 client and", func() {

			It("create a Secret if it does not exist", func() {

				tstName, tstClientID, tstSecretName := makeClientInfo()
				expectedRequest := &reconcile.Request{NamespacedName: types.NamespacedName{Name: tstName, Namespace: tstNamespace}}

				s := scheme.Scheme
				err := hydrav1alpha1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				err = apiv1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
				// channel when it is finished.
				mgr, err := manager.New(cfg, manager.Options{Scheme: s})
				Expect(err).NotTo(HaveOccurred())
				c := mgr.GetClient()

				mch := &mocks.HydraClientInterface{}
				mch.On("GetOAuth2Client", Anything).Return(nil, false, nil)
				mch.On("DeleteOAuth2Client", Anything).Return(nil)
				mch.On("ListOAuth2Client", Anything).Return(nil, nil)
				mch.On("PostOAuth2Client", AnythingOfType("*hydra.OAuth2ClientJSON")).Return(func(o *hydra.OAuth2ClientJSON) *hydra.OAuth2ClientJSON {
					return &hydra.OAuth2ClientJSON{
						ClientID:      &tstClientID,
						Secret:        pointer.StringPtr(tstSecret),
						GrantTypes:    o.GrantTypes,
						ResponseTypes: o.ResponseTypes,
						RedirectURIs:  o.RedirectURIs,
						Scope:         o.Scope,
						Audience:      o.Audience,
						Owner:         o.Owner,
					}
				}, func(o *hydra.OAuth2ClientJSON) error {
					return nil
				})

				recFn, requests := SetupTestReconcile(getAPIReconciler(mgr, mch))

				Expect(add(mgr, recFn)).To(Succeed())

				//Start the manager and the controller
				stopMgr, mgrStopped := StartTestManager(mgr)

				instance := testInstance(tstName, tstSecretName)
				err = c.Create(context.TODO(), instance)
				// The instance object may not be a valid object because it might be missing some required fields.
				// Please modify the instance object by adding required fields and then remove the following if statement.
				if apierrors.IsInvalid(err) {
					Fail(fmt.Sprintf("failed to create object, got an invalid object error: %v", err))
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(*expectedRequest)))

				//Verify the created CR instance status
				var retrieved hydrav1alpha1.OAuth2Client
				ok := client.ObjectKey{Name: tstName, Namespace: tstNamespace}
				err = c.Get(context.TODO(), ok, &retrieved)
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved.Status.ReconciliationError.Code).To(BeEmpty())
				Expect(retrieved.Status.ReconciliationError.Description).To(BeEmpty())

				//Verify the created Secret
				var createdSecret apiv1.Secret
				ok = client.ObjectKey{Name: tstSecretName, Namespace: tstNamespace}
				err = k8sClient.Get(context.TODO(), ok, &createdSecret)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdSecret.Data[controllers.ClientIDKey]).To(Equal([]byte(tstClientID)))
				Expect(createdSecret.Data[controllers.ClientSecretKey]).To(Equal([]byte(tstSecret)))
				Expect(createdSecret.OwnerReferences).To(Equal(getOwnerReferenceTo(retrieved)))

				//delete instance
				c.Delete(context.TODO(), instance)

				//Ensure manager is stopped properly
				close(stopMgr)
				mgrStopped.Wait()
			})

			It("create a Secret if it does not exist and no client secret is generated", func() {

				tstName, tstClientID, tstSecretName := makeClientInfo()
				expectedRequest := &reconcile.Request{NamespacedName: types.NamespacedName{Name: tstName, Namespace: tstNamespace}}

				s := scheme.Scheme
				err := hydrav1alpha1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				err = apiv1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
				// channel when it is finished.
				mgr, err := manager.New(cfg, manager.Options{Scheme: s})
				Expect(err).NotTo(HaveOccurred())
				c := mgr.GetClient()

				mch := &mocks.HydraClientInterface{}
				mch.On("DeleteOAuth2Client", Anything).Return(nil)
				mch.On("ListOAuth2Client", Anything).Return(nil, nil)
				mch.On("PostOAuth2Client", AnythingOfType("*hydra.OAuth2ClientJSON")).Return(func(o *hydra.OAuth2ClientJSON) *hydra.OAuth2ClientJSON {
					return &hydra.OAuth2ClientJSON{
						ClientID:                &tstClientID,
						GrantTypes:              o.GrantTypes,
						ResponseTypes:           o.ResponseTypes,
						RedirectURIs:            o.RedirectURIs,
						Scope:                   o.Scope,
						Audience:                o.Audience,
						Owner:                   o.Owner,
						TokenEndpointAuthMethod: o.TokenEndpointAuthMethod,
					}
				}, func(o *hydra.OAuth2ClientJSON) error {
					return nil
				})

				recFn, requests := SetupTestReconcile(getAPIReconciler(mgr, mch))

				Expect(add(mgr, recFn)).To(Succeed())

				//Start the manager and the controller
				stopMgr, mgrStopped := StartTestManager(mgr)

				instance := testInstance(tstName, tstSecretName)
				err = c.Create(context.TODO(), instance)
				// The instance object may not be a valid object because it might be missing some required fields.
				// Please modify the instance object by adding required fields and then remove the following if statement.
				if apierrors.IsInvalid(err) {
					Fail(fmt.Sprintf("failed to create object, got an invalid object error: %v", err))
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(*expectedRequest)))

				//Verify the created CR instance status
				var retrieved hydrav1alpha1.OAuth2Client
				ok := client.ObjectKey{Name: tstName, Namespace: tstNamespace}
				err = c.Get(context.TODO(), ok, &retrieved)
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved.Status.ReconciliationError.Code).To(BeEmpty())
				Expect(retrieved.Status.ReconciliationError.Description).To(BeEmpty())

				//Verify the created Secret
				var createdSecret apiv1.Secret
				ok = client.ObjectKey{Name: tstSecretName, Namespace: tstNamespace}
				err = k8sClient.Get(context.TODO(), ok, &createdSecret)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdSecret.Data[controllers.ClientIDKey]).To(Equal([]byte(tstClientID)))
				Expect(createdSecret.OwnerReferences).To(Equal(getOwnerReferenceTo(retrieved)))

				//delete instance
				c.Delete(context.TODO(), instance)

				//Ensure manager is stopped properly
				close(stopMgr)
				mgrStopped.Wait()
			})

			It("update object status if the call failed", func() {

				tstName, _, tstSecretName := makeClientInfo()
				expectedRequest := &reconcile.Request{NamespacedName: types.NamespacedName{Name: tstName, Namespace: tstNamespace}}

				s := scheme.Scheme
				err := hydrav1alpha1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				err = apiv1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
				// channel when it is finished.
				mgr, err := manager.New(cfg, manager.Options{Scheme: s})
				Expect(err).NotTo(HaveOccurred())
				c := mgr.GetClient()

				mch := &mocks.HydraClientInterface{}
				mch.On("GetOAuth2Client", Anything).Return(nil, false, nil)
				mch.On("PostOAuth2Client", Anything).Return(nil, errors.New("error"))
				mch.On("DeleteOAuth2Client", Anything).Return(nil)
				mch.On("ListOAuth2Client", Anything).Return(nil, nil)

				recFn, requests := SetupTestReconcile(getAPIReconciler(mgr, mch))

				Expect(add(mgr, recFn)).To(Succeed())

				//Start the manager and the controller
				stopMgr, mgrStopped := StartTestManager(mgr)

				instance := testInstance(tstName, tstSecretName)
				err = c.Create(context.TODO(), instance)
				// The instance object may not be a valid object because it might be missing some required fields.
				// Please modify the instance object by adding required fields and then remove the following if statement.
				if apierrors.IsInvalid(err) {
					Fail(fmt.Sprintf("failed to create object, got an invalid object error: %v", err))
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(*expectedRequest)))

				//Verify the created CR instance status
				var retrieved hydrav1alpha1.OAuth2Client
				ok := client.ObjectKey{Name: tstName, Namespace: tstNamespace}
				err = c.Get(context.TODO(), ok, &retrieved)
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved.Status.ReconciliationError).NotTo(BeNil())
				Expect(retrieved.Status.ReconciliationError.Code).To(Equal(hydrav1alpha1.StatusRegistrationFailed))
				Expect(retrieved.Status.ReconciliationError.Description).To(Equal("error"))

				//Verify no secret has been created
				var createdSecret apiv1.Secret
				ok = client.ObjectKey{Name: tstSecretName, Namespace: tstNamespace}
				err = k8sClient.Get(context.TODO(), ok, &createdSecret)
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())

				//delete instance
				c.Delete(context.TODO(), instance)

				//Ensure manager is stopped properly
				close(stopMgr)
				mgrStopped.Wait()
			})

			It("use provided Secret if it exists", func() {

				tstName, tstClientID, tstSecretName := makeClientInfo()
				var postedClient *hydra.OAuth2ClientJSON
				expectedRequest := &reconcile.Request{NamespacedName: types.NamespacedName{Name: tstName, Namespace: tstNamespace}}

				s := scheme.Scheme
				err := hydrav1alpha1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				err = apiv1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
				// channel when it is finished.
				mgr, err := manager.New(cfg, manager.Options{Scheme: s})
				Expect(err).NotTo(HaveOccurred())
				c := mgr.GetClient()

				mch := mocks.HydraClientInterface{}
				mch.On("GetOAuth2Client", Anything).Return(nil, false, nil)
				mch.On("DeleteOAuth2Client", Anything).Return(nil)
				mch.On("ListOAuth2Client", Anything).Return(nil, nil)
				mch.On("GetOAuth2Client", Anything).Return(nil, false, nil)
				mch.On("PostOAuth2Client", AnythingOfType("*hydra.OAuth2ClientJSON")).Return(func(o *hydra.OAuth2ClientJSON) *hydra.OAuth2ClientJSON {
					postedClient = &hydra.OAuth2ClientJSON{
						ClientID:      o.ClientID,
						Secret:        o.Secret,
						GrantTypes:    o.GrantTypes,
						ResponseTypes: o.ResponseTypes,
						RedirectURIs:  o.RedirectURIs,
						Audience:      o.Audience,
						Scope:         o.Scope,
						Owner:         o.Owner,
					}
					return postedClient
				}, func(o *hydra.OAuth2ClientJSON) error {
					return nil
				})

				recFn, requests := SetupTestReconcile(getAPIReconciler(mgr, &mch))

				Expect(add(mgr, recFn)).To(Succeed())

				//Start the manager and the controller
				stopMgr, mgrStopped := StartTestManager(mgr)

				//ensure secret exists
				secret := apiv1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tstSecretName,
						Namespace: tstNamespace,
					},
					Data: map[string][]byte{
						controllers.ClientIDKey:     []byte(tstClientID),
						controllers.ClientSecretKey: []byte(tstSecret),
					},
				}
				err = c.Create(context.TODO(), &secret)
				Expect(err).NotTo(HaveOccurred())

				instance := testInstance(tstName, tstSecretName)
				err = c.Create(context.TODO(), instance)
				// The instance object may not be a valid object because it might be missing some required fields.
				// Please modify the instance object by adding required fields and then remove the following if statement.
				if apierrors.IsInvalid(err) {
					Fail(fmt.Sprintf("failed to create object, got an invalid object error: %v", err))
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(*expectedRequest)))

				//Verify the created CR instance status
				var retrieved hydrav1alpha1.OAuth2Client
				ok := client.ObjectKey{Name: tstName, Namespace: tstNamespace}
				err = c.Get(context.TODO(), ok, &retrieved)
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved.Status.ReconciliationError.Code).To(BeEmpty())
				Expect(retrieved.Status.ReconciliationError.Description).To(BeEmpty())

				Expect(*postedClient.ClientID).To(Equal(tstClientID))
				Expect(*postedClient.Secret).To(Equal(tstSecret))

				// Ensure that secret doesn't have OwnerReference set
				ok = client.ObjectKey{Name: tstSecretName, Namespace: tstNamespace}
				err = k8sClient.Get(context.TODO(), ok, &secret)
				Expect(err).To(BeNil())
				Expect(len(secret.OwnerReferences)).To(Equal(0))

				//delete instance
				c.Delete(context.TODO(), instance)

				//Ensure manager is stopped properly
				close(stopMgr)
				mgrStopped.Wait()
			})

			It("update object status if provided Secret is invalid", func() {

				tstName, tstClientID, tstSecretName := makeClientInfo()
				expectedRequest := &reconcile.Request{NamespacedName: types.NamespacedName{Name: tstName, Namespace: tstNamespace}}

				s := scheme.Scheme
				err := hydrav1alpha1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				err = apiv1.AddToScheme(s)
				Expect(err).NotTo(HaveOccurred())

				// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
				// channel when it is finished.
				mgr, err := manager.New(cfg, manager.Options{Scheme: s})
				Expect(err).NotTo(HaveOccurred())
				c := mgr.GetClient()

				mch := mocks.HydraClientInterface{}
				mch.On("GetOAuth2Client", Anything).Return(nil, false, nil)
				mch.On("DeleteOAuth2Client", Anything).Return(nil)
				mch.On("ListOAuth2Client", Anything).Return(nil, nil)

				recFn, requests := SetupTestReconcile(getAPIReconciler(mgr, &mch))

				Expect(add(mgr, recFn)).To(Succeed())

				//Start the manager and the controller
				stopMgr, mgrStopped := StartTestManager(mgr)

				//ensure invalid secret exists
				secret := apiv1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tstSecretName,
						Namespace: tstNamespace,
					},
					Data: map[string][]byte{
						controllers.ClientIDKey: []byte(tstClientID),
						//missing client secret key
					},
				}
				err = c.Create(context.TODO(), &secret)
				Expect(err).NotTo(HaveOccurred())

				instance := testInstance(tstName, tstSecretName)
				err = c.Create(context.TODO(), instance)
				// The instance object may not be a valid object because it might be missing some required fields.
				// Please modify the instance object by adding required fields and then remove the following if statement.
				if apierrors.IsInvalid(err) {
					Fail(fmt.Sprintf("failed to create object, got an invalid object error: %v", err))
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Eventually(requests, timeout).Should(Receive(Equal(*expectedRequest)))

				//Verify the created CR instance status
				var retrieved hydrav1alpha1.OAuth2Client
				ok := client.ObjectKey{Name: tstName, Namespace: tstNamespace}
				err = c.Get(context.TODO(), ok, &retrieved)
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved.Status.ReconciliationError).NotTo(BeNil())
				Expect(retrieved.Status.ReconciliationError.Code).To(Equal(hydrav1alpha1.StatusInvalidSecret))
				Expect(retrieved.Status.ReconciliationError.Description).To(Equal(`"client_secret property missing"`))

				//delete instance
				c.Delete(context.TODO(), instance)

				//Ensure manager is stopped properly
				close(stopMgr)
				mgrStopped.Wait()
			})
		})
	})
})

func getOwnerReferenceTo(c hydrav1alpha1.OAuth2Client) []metav1.OwnerReference {
	return []metav1.OwnerReference{{
		APIVersion: c.APIVersion,
		Kind:       c.Kind,
		Name:       c.Name,
		UID:        c.UID,
	}}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("api-gateway-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Api
	err = c.Watch(&source.Kind{Type: &hydrav1alpha1.OAuth2Client{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

func getAPIReconciler(mgr ctrl.Manager, mock controllers.HydraClientInterface) reconcile.Reconciler {
	return &controllers.OAuth2ClientReconciler{
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("OAuth2Client"),
		HydraClient: mock,
		HydraClientMaker: func(hydrav1alpha1.OAuth2ClientSpec) (controllers.HydraClientInterface, error) {
			return mock, nil
		},
	}
}

func testInstance(name, secretName string) *hydrav1alpha1.OAuth2Client {

	return &hydrav1alpha1.OAuth2Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: tstNamespace,
		},
		Spec: hydrav1alpha1.OAuth2ClientSpec{
			GrantTypes:    []hydrav1alpha1.GrantType{"client_credentials"},
			ResponseTypes: []hydrav1alpha1.ResponseType{"token"},
			Scope:         "a b c",
			RedirectURIs:  []hydrav1alpha1.RedirectURI{"https://example.com"},
			Audience:      []string{"audience-a"},
			SecretName:    secretName,
			HydraAdmin: hydrav1alpha1.HydraAdmin{
				URL:            "http://hydra-admin",
				Port:           4445,
				Endpoint:       "/client",
				ForwardedProto: "https",
			},
		}}
}
