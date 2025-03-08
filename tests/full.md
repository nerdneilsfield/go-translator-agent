# A Real-Time FPGA-Based Point Cloud Registration Framework with Ultra-Fast and Confgurable Correspondence Search  

Qi Deng, Graduate Student Member, IEEE, Hao Sun, Member, IEEE, Yuhao Shu, Jianzhong Xiao, Weixiong Jiang, Hui Wang and Yajun Ha, Senior Member, IEEE  

Abstract—Point cloud registration is a critical component of LiDAR-based localization and mapping systems, yet existing implementations can only achieve limited speed due to the ineffciency of correspondence search. To address this challenge, we propose a real-time FPGA-based point cloud registration framework with ultra-fast and confgurable correspondence search capabilities. This framework incorporates three key innovations: First, we develop a novel Range-Projection Structure (RPS) that organizes unstructured LiDAR points into a matrix-like format, enabling effcient point localization and grouping nearby points into continuous memory segments to accelerate access. Second, we introduce an effcient multi-mode correspondence search algorithm, leveraging the RPS structure to narrow the search region, eliminate redundant points, and support various types of correspondences by incorporating LiDAR-specifc laser channel information. Third, we design a confgurable ultra-fast RPSbased correspondence search (RPS-CS) accelerator, featuring a high-performance RPS-Builder for rapid structure construction and a highly parallel RPS-Searcher for fast correspondence search. The accelerator is further enhanced with a dynamic caching strategy and a pipeline batcher module to improve effciency and confgurability. Experimental results show that the RPS-CS accelerator achieves a $7.5\times$ speedup and $17.1\times$ energy effciency improvement over state-of-the-art FPGA implementations, while the proposed framework achieves real-time performance for 64-laser LiDAR data with a frame rate of 20.1 FPS.  

Index Terms—FPGA, hardware acceleration, correspondence earch, point cloud registration, LiDAR-based SLAM systems  

# I. INTRODUCTION  

Point cloud registration is a crucial component of the LiDAR-based Simultaneous Localization and Mapping (LSLAM) system, which involves calculating a transformation matrix to align the source point cloud with the target point cloud, as illustrated in Fig. 1. The fowchart of the state-ofthe-art registration algorithm [1] is given in Fig. 2, which receives two types of feature points and computes the optimal transformation matrix. Effcient and accurate registration implementation has always been a prominent research topic in many felds, such as autonomous driving and robotics.  

To enhance the registration algorithm’s accuracy, it is imperative to employ an advanced LiDAR system with more laser channels and higher frame rates. For instance, the 3D LiDAR (Robosense Ruby Plus) has been upgraded to 128 lasers with a scanning frequency of $20\mathrm{Hz}$ , generating 4,608,000 points per second. However, this upgrade will inevitably lead to a substantial increase in the execution time of the registration algorithm [2] [3]. The profling results shown in Fig. 3 reveal that even with point clouds from a 64-laser LiDAR, the registration algorithm [1] can only achieve a processing speed of less than 2 frames per second (FPS) on a vehicleembedded CPU, which falls signifcantly short of meeting the real-time requirement. The performance bottleneck falls at the correspondence searching task, which constitutes $91.6\%$ of the overall registration time.  

Previous works have made signifcant efforts to accelerate correspondence search in point cloud registration by optimizing search structures, search algorithms, and hardware accelerators. Several studies [4]–[6] organize sparse and uneven point clouds into tree-based or voxel-based search structures, enabling effcient K-Nearest-Neighbor correspondence (KNNC) searches. However, these structures often fail to balance rapid point localization and effcient extraction of neighboring points, especially for highly uneven point distributions. Additionally, they struggle to retain critical 3D structural and geometric information, limiting their ability to handle geometric correspondence such as Plane-NN (PNN-C) and Edge-NN (ENN-C). Search algorithms also face challenges in achieving parallelism and effciently processing geometric correspondence. Approximate methods [7]–[10] reduce search time but compromise accuracy, while data access optimizations [11], [12], such as caching and parallel aggregation, improve effciency but add overhead for reordering and scheduling operations, limiting their real-time applicability. Hardware accelerators, including GPUs and FPGAs [13]–[15], further enhance performance. GPUs leverage parallelism to accelerate KDtree construction and KNN search but suffer from high energy consumption, reducing energy effciency. FPGA-based solutions [5], [6] are more energy-effcient, employing cooptimization strategies to divide tasks between hardware and software. However, they often fail to handle large-scale data caching effciently or provide confgurable support for diverse geometric correspondences like PNN-C and ENN-C, which are critical for point cloud registration.  

![](images/9d3da5e7f79eca50495723906b3d449d3dec80312eb2192ca2754f2c26ebbff3.jpg)  
Fig. 1. Illustration of point cloud registration algorithm.  

To solve these issues, we propose an FPGA-based real-time registration framework for LiDAR point clouds equipped with ultra-fast and confgurable correspondence search capabilities. We have made the following three contributions.  

• A novel search structure (RPS) for effcient query point localization and neighboring points access. The RPS structure organizes the unstructured and unevenly distributed LiDAR points into a matrix-like format based on their projection locations and range values (distance to the LiDAR). To facilitate rapid access to neighboring points, the RPS groups points with similar range values and projection locations into continuous memory segments, signifcantly reducing search complexity.  

• An effcient and multi-mode correspondence searching algorithm leveraging the RPS structure. Utilizing the spatial organization provided by RPS, the proposed algorithm effciently narrows down the search region and eliminates a substantial number of redundant points. Moreover, by incorporating LiDAR-specifc laser channel information, the algorithm supports multi-mode correspondence searching, enabling fast and accurate searching for different types of correspondence.  

• An ultra-fast and confgurable RPS-based correspondence search accelerator (RPS-CS). The RPSCS framework consists of two key components: (1) RPS-Builder: A high-performance accelerator designed to rapidly construct the RPS structure from LiDAR point cloud. (2) RPS-Searcher: A highly parallel and confgurable accelerator for fast correspondence searching. To boost memory access effciency, we design a dynamic RPS caching strategy that adaptively preloads neighboring points from external memory into on-chip memory. Furthermore, to enhance search effciency and confgurability, the RPS-Searcher employs a pipeline batcher module to aggregate variable numbers of points into fxed-size batches.  

![](images/740f908ac4dd0dd77adc3e3d2c0189295350bd4d1fa97f82d3bacf06e099293b.jpg)  
Fig. 2. Flowchart of the point cloud registration algorithm.  

![](images/90575386dc1ef68fb8d44cceea239ae4461330e63c614e01e7a8645a12e43963.jpg)  
Fig. 3. Running time profling results of the registration algorithm on CortexA53.  

The remainder of this paper is organized as follows. Section II presents the background knowledge and related work of the point cloud registration algorithm. Section III shows an overview of the proposed framework. Section IV introduces the RPS structure and the building accelerator. Section V shows the fast and scalable correspondence search algorithm and accelerator based on RPS. Section VI gives the experimental results and analysis. Finally, section VII presents the conclusion and future work.  

# II. BACKGROUND AND RELATED WORK  

# A. Defnition and Analysis of the Registration Algorithm  

In the registration task of the L-SLAM system, the current frame’s point cloud is defned as the source point cloud, denoted $\mathbf{P}$ , and the point cloud from the previous frame is defned as the target point cloud, denoted Q, where P, $\mathbf{Q}\in\mathbb{R}^{3}$ . Typically, a frame’s point cloud refers to an assembly of 3D points captured by a LiDAR sensor during its 360-degree horizontal rotational scan.  

The goal of registration is to estimate a rigid motion via the transformation matrix $T=(R,t)$ , thereby maximizing the congruence between the transformed source point cloud $T\cdot P$ and the target point cloud Q. Here, $R$ represents the rotation matrix, while $t$ is the translation vector. Fig. 1 illustrates the registration algorithm.  

Fig. 2 gives the fowchart of the state-of-the-art registration algorithm [1], which receives two types of feature points and computes the optimal transformation matrix. In this context, feature points associated with planes and edges within the source point cloud are designated as $P_{\mathcal{H}}$ and $P_{\mathcal{E}}$ , respectively. Correspondingly, feature points relating to planes and edges in the target point cloud are denoted as $Q_{\mathcal{H}}$ and $Q_{\mathcal{E}}$ , respectively.  

![](images/5621356d277ccf8ea5b63b304c83ceab5b05749ab194f2e567adf35f8e79f8a7.jpg)  
Fig. 4. Illustration of the correspondence searching. The three red points with blue circle comprise the corresponding points of the plane query point $P_{\mathcal{H}}^{i}$ . The two green points with blue circle comprise the corresponding pointsH of the edge query point $P_{\mathcal{E}}^{i^{\prime}}$ .  

Generally, the plane points are located on the wall, while the edge points are located at the corner, as depicted in Fig. 4. The registration algorithm comprises three primary modules.  

1) Build Search Structure Module: The frst module focuses on constructing a search structure to effciently organize the target point clouds, enabling rapid and accurate correspondence searches in subsequent modules. A commonly used search structure in SLAM algorithms is the K-Dimensional Tree (KDtree), a space-partitioning technique that organizes point clouds into a hierarchical tree format to facilitate fast search operations. Consequently, many previous works, such as [1], [16], [17], adopt KDtree to structure both $Q_{\mathcal{H}}$ and $Q_{\mathcal{E}}$ feature points. We denote $N_{Q}$ as the number of points in the target point cloud used for building the search structure.  

2) Search Correspondence Module: The second module of the system is designed to establish correspondences between the source and target point clouds. The quality of the correspondences directly infuences the registration algorithm’s accuracy. Utilizing the KDtree search structure developed in the previous module, [1] searches for correspondence with the following three steps.  

First, transform the points in the source point cloud to the coordinate system of the target point cloud by an initial estimate of transformation matrix $T_{i n i t}\,=\,(R_{i n i t},t_{i n i t})$ , where $R_{i n i t}$ and $t_{i n i t}$ represent the rotation matrix and translation tvreacntsofro. rImne de qtuoa (a1n),d f,e raetusrpee ctpiovienltys. $P_{\mathcal{H}}^{i}$ atrnadn 𝑃𝑖 maerde $P_{\mathcal{E}}^{i}{}^{'}$ source points are defned as the query point, and the number of query points is denoted as $N_{P}$ .  

$$
{P_{\mathcal{E}(\mathcal{H})}^{i}}^{\prime}=T_{i n i t}\cdot P_{\mathcal{E}(\mathcal{H})}^{i}=R_{i n i t}P_{\mathcal{E}(\mathcal{H})}^{i}+t_{i n i t}
$$  

Second, search the correspondence for each query point. As illustrated in Fig. 2, correspondence can be categorized into three types: KNN-C, PNN-C, and ENN-C.  

1) The KNN-C typically involves identifying the K nearest neighbors as the corresponding points for each query point, which is a fundamental function and widely used in robotic applications.   
2) The PNN-C refers to identifying the nearest three plane feature points as the corresponding plane for query plane  

point. In Fig. 4, the PNN-C points of query plane point $P_{\mathcal{H}}^{i}$ are comprised of $(Q_{\mathcal{H}}^{j},\;\stackrel{\cdot}{Q}_{\mathcal{H}}^{m},\;Q_{\mathcal{H}}^{l})$ , where $Q_{\mathcal{H}}^{j}$ represents the nearest point to $P_{\mathcal{H}}^{i}$ in $Q_{\mathcal{H}},\,Q_{\mathcal{H}}^{m}$ represents the nearest point to $P_{\mathcal{H}}^{i}$ located at the neighboring two laser channels of $Q_{\mathcal{H}}^{j^{\dagger}},$ and $Q_{\mathcal{H}}^{l}$ represents the second nearest point to 𝑃𝑖 located at the same laser channel of $Q_{\mathcal{H}}^{j}.$ The PNN-C points $(Q_{\mathcal{H}}^{j},\,Q_{\mathcal{H}}^{m},\,Q_{\mathcal{H}}^{l})$ determines the corresponding plane of 𝑃𝑖 ′, as illustrated by the blue triangle.  

3) The ENN-C refers to identifying the nearest two edge points as the corresponding edge line for query edge point. In Fig. 4, the ENN-C points of query edge point $\mathbf{\bar{}{}}_{P_{\mathcal{E}}^{i}}^{\prime}$ are comprised of $(Q_{\varepsilon}^{j},\,\bar{Q}_{\varepsilon}^{m})$ , which determines the corresponding edge line of 𝑃𝑖 ′, as depicted by the blue line.  

Third, calculate the corresponding distances between the source feature points and their matches. The distances to their associated edge lines and planes are denoted as $\mathbf{d}_{\mathcal{E}}^{i}$ and $\mathrm{d}_{\mathcal{H}}^{i},$ respectively, as illustrated in Fig. 4. It is worth noting that only neighbors within a specifc range are considered meaningful. If the distance between the nearest neighbor and the query point exceeds a predefned threshold, the query point is treated as an outlier and excluded, ensuring it does not impact the registration result. This threshold range, defned as $r_{i n}$ , guarantees that all relevant neighbors are included, while points beyond $r_{i n}$ are ignored.  

3) Estimate Motion Module: The third module focuses on motion estimation using correspondences. The process begins by formulating a nonlinear least-squares function to measure the registration error and employs the Levenberg-Marquardt algorithm to iteratively refne the initial transformation matrix $T_{i n i t}$ into the optimal matrix $T_{o p t}$ . This optimal matrix ensures maximum alignment between the transformed source point cloud (expressed as $T_{o p t}\cdot P)$ and the target point cloud $Q$ , as illustrated in Fig. 1. Additionally, as described in [1], the iteration count is set to 2, which means that each registration process involves building the search structure once and performing correspondence searches twice.  

# B. Related Work  

In this subsection, we introduce the related works of point cloud registration algorithms in terms of search structure optimization, search algorithm improvement, and hardware acceleration.  

In terms of search structure optimization, tree structures, notably KDtree, locality-sensitive hashing, and overfow tree, are commonly employed in point cloud registration. According to a comparative study [9], KDtree offers signifcant advantages in accuracy, build time, search time, and memory usage. However, when dealing with sparse and uneven point clouds typical in autonomous driving scenarios, the effciency of both building and searching processes signifcantly declines. To effectively manage such point clouds, novel spacepartitioned structures such as Double-Segmentation-VoxelStructure (DSVS) [5] and Occupation-Aware-Voxel-Structure (OAVS) [6], [12], [18] have been developed. These methods segment the point cloud into 3D cubic spaces, or voxels, eliminating empty voxels and continuously segmenting the occupied ones, subsequently organizing points by their voxel’s hash value. Although these approaches demonstrate competitive advantages in building time and memory usage, the variable number of points in each voxel and the variable number of neighboring voxels results in relatively slower search speeds.  

In terms of search algorithm optimization, several studies [7], [11], [19], [20] adopt approximate search methods, such as range search or probability distribution search, which substantially reduces search time by up to two orders of magnitude. However, these methods tend to compromise accuracy and robustness. To improve search effciency without sacrifcing accuracy, some researches [12], [21]–[23] focus on optimizing data access strategies. Techniques like caching search results to enhance the hit rate in subsequent searches or aggregating points for parallel access have been explored. However, these methods introduce additional time requirements for reordering or scheduling the points. Furthermore, it is essential to leverage specifc features inherent in the correspondence search task, notably the locality and geometry of corresponding points, to decisively enhance search effciency.  

In terms of hardware acceleration, FPGAs and GPUs have been widely adopted to accelerate registration algorithms. Studies [13], [24], [25] demonstrate effcient KD-tree construction and KNN correspondence search on GPUs using highly parallel strategies. However, GPU-based approaches often suffer from high energy consumption, reducing their energy effciency. In contrast, FPGA-based solutions offer greater energy effciency. Several works [5], [8], [9], [26]–[28] employ reusable multilevel caches, keyframe-based scheduling, and highly parallel sorting and selection circuits to improve real-time performance. Despite these advances, most efforts focus on optimizing search accelerators while neglecting the execution time of other components and data transfer overhead, limiting overall effciency. To address these issues, some studies [6], [15] divide registration algorithms into hardware and software components, leveraging co-optimization strategies to reduce redundant search operations and improve effciency. Meanwhile, other works [10], [14], [29] optimize both structure-building and KNN search at algorithmic, architectural, and cache levels, enabling highly confgurable and ultra-fast KNN accelerators. However, most rely on approximate search techniques that discard many neighboring points, failing to meet the stringent accuracy requirements of LSLAM systems. Additionally, nearly all existing approaches fail to incorporate geometric properties of the search structure, leading to ineffcient plane and edge correspondence searches.  

# III. OVERVIEW OF THE PROPOSED REGISTRATION FRAMEWORK  

This section provides an overview of the proposed RPSCS based software-hardware co-design registration framework. Building on the analysis in Section II-A, we introduce the key components of the framework, including the RPS structure, the RPS-based correspondence search algorithm, the RPS-CS accelerator, and the collaborative registration workfow.  

![](images/baccb50115b09e64e897178f2356b7d6dd3dee1001674ac045901684583f8bb1.jpg)  
Fig. 5. Illustration of segmenting the point cloud by the RPS structure. Points with the same color are measured from identical laser channels.  

# A. Overview to RPS structure and RPS-CS algorithm  

To exploit the locality and geometric features of corresponding points, we propose a novel search structure called RPS, as illustrated in Fig. 5. The RPS structure organizes point cloud data into an effcient format for correspondence search through the following workfow:  

• First, the RPS structure projects the point cloud into a matrix format, where rows correspond to laser channels and columns to horizontal rotation angles. This process creates a structured representation of the data, where the blue grids in Fig. 5 indicate the projection locations. • Second, the projected points are segmented into range scales based on their distance to the LiDAR sensor (range value). Each range scale corresponds to a specifc interval of range values, allowing points to be precisely located using row, column, and range scale indices. Fig. 5 illustrates the relationship between range values and range scales. • Third, we segment the point cloud matrix into a set of range scale segmentation domain (RSSD), where points with similar range scales are reordered into contiguous memory blocks with a counting sort algorithm. The resulting RPS structure consists of two key components: RPS-Points, which store the reordered points, and RPSIndex, which records the starting index of each range scale. Fig. 7 provides a detailed example of this structure.  

Building on the RPS structure, we propose an effcient and fexible correspondence search algorithm, referred to as RPSCS, to identify various types of corresponding points within a specifed search radius $r_{i n}$ . The RPS-CS algorithm is executed in the following fve steps:  

• First, the RPS position of the query point is determined by calculating its row and column indices based on its horizontal angle and laser channel, and its range scale index based on its range value. The range scale index is obtained using a pre-defned look-up table (LUT). Second, the search region is determined based on the query point’s location and the search radius $r_{i n}$ , as equation 5 shows. This region is represented as a set of  

![](images/4f7d464d8f5cba0acbc661dba52d5dbb8318a07e3b3e48faa6c231754cd6c77b.jpg)  
Fig. 6. Illustration of the proposed RPS-CS accelerator and the RPS-CS based software-hardware co-design registration framework.  

RPS-Index pairs, where each pair specifes a subset of points in the RPS-Points.  

• Third, candidate corresponding points are extracted from the RPS structure. For each valid RPS-Index pairs, the points are parallel retrieved from RPS-Points because the points with similar range scales and projection locations have been reordered into contiguous memory blocks. • Fourth, the algorithm identifes the KNN-C from the candidate points using a highly parallel K-selection method. If the search objective is limited to KNN-C, the results are directly returned. Otherwise, the candidate points are further fltered for specifc geometric features. Finally, the fltered points are used to search for additional types of correspondences, such as PNN-C or ENN-C. These additional correspondences are computed by a fast laser-channel based conditional K-selection method, as shown in Algorithm 2.  

# B. Overview of the RPS-CS based registration framework  

In addition to optimizing the search structure and search algorithm, we also present a software-hardware co-designed registration framework to further boost the performance. This framework is built upon a heterogeneous system architecture, combining a high-performance Processing System (PS) with user-Programmable Logic (PL) on a single FPGA board. The structural design and operational workfow of the framework are illustrated in Fig. 6.  

On the PL side, we implement an RPS-CS accelerator comprising two main components: the RPS-Builder and the RPS-Searcher.  

• RPS-Builder: This component is responsible for constructing the RPS structure using a datafow architecture, represented by the blue blocks in Fig. 6. For points in each RSSD, the modules handle the following tasks: projecting target points into a matrix format, organizing points into RSSD-wise streams, counting points within each range scale, calculating the starting index for each range scale, and reordering points accordingly. This parallelized process ensures effcient RPS structure generation.  

• RPS-Searcher: This component is designed to perform fast and confgurable correspondence searches with a focus on memory effciency and parallelism, represented by the green blocks in Fig. 6. The search process is organized into seven modules: projecting the query point into the RPS structure, narrowing the search region, extracting RPS-Index pairs, parallel extracting points within each pair, aggregating near points into batches, employing highly parallel K-selection circuits to search for the KNNC, and using laser-channel-based K-selection circuits to identify PNN-C and ENN-C. This modular design ensures high effciency and adaptability for various correspondence search tasks.  

Besides, the RPS-Parameter component confgures all the modules in the RPS-CS accelerator to support multi-mode operations and custom optimization functions. It enables switching between the RPS-Builder and RPS-Searcher components and adjusts parameters such as search region size based on different correspondence types.  

On the PS side, the framework manages point cloud data storage, RPS structure information, motion estimation, and the overall control of the collaborative registration workfow, as described in Section II-A.  

All interfaces between the PS and PL sides are implemented using streaming FIFO ports, with data packing applied to enhance transfer effciency. Within the accelerator, data is represented in fxed-point format, while outside the accelerator, it is in foating-point format. The data type conversion process is illustrated by the port icons in Fig. 6. Moreover, The RegisterTransfer Level (RTL) model for the FPGA is generated by the Xilinx Vitis high-level synthesis (HLS) tool. Besides, we have incorporated a dynamic voltage scaling module [30] to optimize the energy effciency.  

# IV. EFFECTIVELY BUILD THE RPS SEARCH STRUCTURE  

In this section, we frst introduce the details of building the RPS structure. Then, we propose the hardware implementation of the RPS-Builder accelerator.  

![](images/2f26f236b1bcb500c0aa0bcc88e1ee28611c897a4106d2d802a81d424951c1ba.jpg)  
(c) Derive the RPS structure by the counting sort algorithm   
Fig. 7. An example of building RPS search structure. The ‘R’ represents the range scale.  

# A. Build the RPS Search Structure  

Since the RPS search structure segments the point cloud based on projection locations and range values, there are three stages to build the RPS structure. First, segment the point cloud by projecting the points into a matrix. As Fig. 7 (a) shows, the raw point cloud is organized into a RSSDwise matrix, utilizing a low-complexity yet precise projection method [30]. Second, segment the point cloud matrix by range value. We traverse the point cloud matrix in a RSSD-wise manner to calculate the range value for each point. These range values represent the distance from each point to the LiDAR sensor. Based on these values, we segment the points within each pre-defned domain into different range scales, as Fig. 7 (b) shows. Third, derive the RPS search structure by a counting sort algorithm. The RPS structure comprises two key components: the reordered points (RPS-Points) and the frst index of each range scale (RPS-Index). The numerical difference between adjacent RPS-Index values indicates the count of points within each respective range scale. Fig. 7 (c) gives an example of the RPS-Points and RPS-Index.  

1) Segment the point cloud by projecting the points into a matrix: Considering that a frame point cloud is captured by a LiDAR sensor during a 360-degree horizontal rotation scan, it is feasible to project the point cloud into a matrix of dimensions $V\times H$ . Here, $V$ represents the number of laser channels, and $H$ denotes the count of measurements obtained from each laser channel throughout the 360-degree horizontal rotation. The value of $H$ is calculated by the formula $H\,=$ $\frac{360}{\Delta\alpha}$ , where $\Delta\alpha$ denotes the angle resolution of the horizontal rotation.  

Given a point $p(x,y,z)$ , the row and column indices $(\nu,h)$ are calculated by equation (2), where $\Delta\omega$ is the average angular resolution between consecutive laser channels in the vertical direction.  

<html><body><table><tr><td>Algorithm 1 Obtain RPS-Points by counting sort algorithm</td></tr><tr><td>Input: Point cloud matrix PCM[H][V] Input:RPS-Index RPSI[H][M] Output: RPS-Points RPSP[No]</td></tr><tr><td>1: reset range scale occupancy count RSOC[H][M] = 0 2:reset reorderindexRI=0 3: for i ∈ [O,H) do >eachRSSDinPCM</td></tr><tr><td>4: for j ∈ [o, V) do > each point in RSSD</td></tr><tr><td>5: obtain the range scale R of PCMij</td></tr><tr><td>6: if R ∈ [o, M) then > filter empty elements</td></tr><tr><td>7: RI ←RPSI[i][R] + RSOC[i][R]</td></tr><tr><td>8: RSOC[i][R]←RSOC[i][R]+ 1 9: RPSP[RI] ← PCMij 10: end if 11: end for 12: end for</td></tr></table></body></html>  

$$
\begin{array}{l}{\nu=a r c t a n(z/\sqrt{(x^{2}+y^{2})})/\Delta\omega}\\ {h=a r c t a n(y/x)/\Delta\alpha}\end{array}
$$  

To effciently calculate $(\nu,h)$ , we adopt an improved methodology based on our previous research [30], which utilizes two LUTs to swiftly and accurately determine the row and column indices. Initially, the vertical and horizontal lookup values, denoted as $a_{\nu}$ and $a_{h}$ , are calculated using the equations $a_{\nu}=z^{2}/(x^{2}+y^{2})$ and $a_{h}=y/x$ , respectively. These values are then used to retrieve the corresponding row and column indices from two pre-defned LUTs. The LUTs are specifcally designed to account for the vertical angles of the laser channels and the horizontal rotational angles, ensuring both accuracy and computational effciency.  

Fig. 7 (a) gives an example of a point cloud matrix, where the coordinates $(\nu,h)$ of the yellow triangle are determined to be (5, 2).  

2) Segment the point cloud by range value: Considering the sparse and uneven distribution of LiDAR points, which are typically denser in nearer ranges and sparser in farther ranges, we incorporate the concept of range scale to further segment the point cloud matrix. This process involves two primary steps:  

$$
r=\sqrt{x^{2}+y^{2}+z^{2}}
$$  

Calculation of range value: Given a point $p(x,y,z)$ , we frst calculate the range value $r$ by equation (3). This value quantifes the distance from point $p$ to the LiDAR sensor.  

Segmentation using range scales: Next, we introduce range scales to segment the range space into several intervals. Assuming the LiDAR’s maximum detection range is $r_{m a x}$ meters, we segment the range space into $M$ range scales. The value of $r_{m a x}$ is typically obtained from the LiDAR’s datasheet, while $M$ is determined based on a combination of the point cloud distribution characteristics and experimental analysis. Subsequently, an uneven range scale LUT (RLUT) is established to associate different range values with their corresponding range scales. Fig. 5 illustrates the relationship between the range value $(r)$ and the range scale $(R)$ , where the maximum range value $(r_{m a x})$ is set to 20 meters and $M$ equals 4. Each $R_{i}$ corresponds to the interval $[r_{i},\,r_{i+1})$ . In Fig. 7 (a), the range scale $R$ of the yellow triangle is 3.  

![](images/a2df41529cc25aa10aca583fe0e05b426eb1f5978080e464fc348fa3a72d3599.jpg)  
Fig. 8. Hardware architecture of the RPS-Builder accelerator.  

Therefore, we can obtain an enhanced point cloud matrix with attributes $(x,y,z,r,R)$ . Besides, the empty elements are flled with $(0,0,0,-1,-1)$ .  

3) Derive the RPS Search Structure: To construct the RPS search structure, we utilize a streaming counting sort method based on the point cloud matrix and range scales. First, the RSSD is defned to segment the point cloud matrix into domains, with each RSSD encompassing $N_{s d}$ columns. Fig. 7 (a-b) highlights eight distinct RSSDs, each in a different color. Second, the counting sort method is applied to each RSSD in three steps:  

1) Count the number of points in each range scale. The counting results for the frst three RSSDs are shown in Fig. 7 (c).   
2) Compute the RPS-Index, which indicates the starting position of points for each range scale in the reordered RPS-Points. This is achieved by cumulatively summing the counts, as depicted in Fig. 7 (c).   
3) Reorder the points to generate RPS-Points based on the range scale and RPS-Index. Algorithm 1 details this process, and Fig. 7 (c) provides an example.  

The resulting reordered target point cloud, termed RPSPoints, retains the same size as the original but is characterized by $(x,y,z,r,R,\nu)$ . The RPS-Index size is $M\!\times\!H/N_{s d}$ , refecting the segmentation of the point cloud by range scales and RSSDs.  

Special Process for ENN-C: Edge feature points are required to exhibit higher curvature. As described in [30], the distance between two consecutive edge feature points obtained from the same scanning laser beam is greater than 5. To optimize the ENN-C search structure, we adjust the size of the mapped point cloud matrix to $M\times H/N_{s d}/5$ . This modifcation signifcantly reduces the search space during the ENN-C search process, improving computational effciency.  

# B. Hardware Accelerator of Building the RPS  

In this subsection, we introduce the hardware design and optimization strategies implemented in our proposed RPSBuilder accelerator. We focus on enhancing the performance while concurrently reducing the hardware resource utilization.  

At the architecture level, we design an effcient task-level pipeline structure for the accelerator, as illustrated by the red line in Fig. 8. The process starts by projecting the target points into a matrix format. These points are then scheduled in an RSSD-wise order and subsequently reordered into the RPSPoints array.  

To simplify the architecture fgure, we set the parameters as $M\,=\,72$ , $H\,=\,1800$ , and $V\;=\;64$ . Additionally, the RCounter and R-First-Index modules are merged into the PointsReorder module. Consequently, the RPS-Builder accelerator is divided into three distinct modules: Points-Projection (PP), RSSD-Wise Scheduler (RWS), and Points-Reorder (PR).  

1) Points-Projection Module: This module is designed to optimize the hardware resource effciency while sustaining a high-throughput pipeline architecture. We employ a multiresolution LUT based strategy to facilitate this balance, as illustrated in Fig. 8.  

To effciently look up the column index in a lengthy LUT of 1800 entries, we utilize a coordinate-based, multi-resolution LUT method comprising four key steps. First, we compute the look-up value $a_{h}$ to measure the horizontal angle of each point using a divider. Second, we employ a compact, 4-entry coordinate axis LUT to identify the quadrant of the horizontal angle by $x$ and $y$ value. Third, we refne the look-up region by $a_{h}$ using a coarse-grained 30-entry LUT. Finally, within this reduced region, a fne-grained LUT is conducted to look up the column index $h$ .  

Considering the range scale LUT and the row LUT are relatively smaller in size, we implement separate, isolated LUTs for their respective functionalities. The sizes of those two LUTs are confgured to be 72 and 64, respectively, which correspond to the number of range scales and laser channels.  

2) RSSD-Wise Scheduler Module: Despite employing a high-precision LUT based projection method, the ordering of projected points is not strictly adherent to an RSSD-wise sequence. The out-of-order and scattered points compromises the effciency of our accelerator. Therefore, we have developed an effcient RWS module that utilizes a compact point cloud matrix buffer (PCMB) and a high-throughput pipeline architecture to ensure strict adherence to RSSD-wise order in the output of points. The implementation faces three primary challenges:  

• Determining the size of the PCMB: Extensive experimental analysis led us to set the PCMB column size to 8. This size is optimal to prevent overwriting of incoming points on those yet to be outputted.   
• Pipeline scheduling of PCMB input and output: We dynamically manage the PCMB’s read and write operations by comparing the write and read columns, as indicated by the green line in Fig. 8. This mechanism halts the input of new points when the output queue is overloaded, thus maintaining the high performance of the pipeline structure.   
• Preventing overwriting of the points to be outputted: A 1-bit Projection Flag (PF) array, sized $64\,\times\,1800$ , is implemented to track whether an element has been projected. If projected, the data in the PCMB is outputted; otherwise, a default data with a range scale of -1 is outputted. Additionally, as the blue line in Fig. 8 illustrates, we incorporate a PF array reset process within the pipeline structure to optimize the latency. This process continuously resets elements in the 4 to 8 columns following the input column.  

3) Points-Reorder Module: This module effectively implements the counting sort algorithm using a high-throughput pipeline structure. As illustrated in Fig. 8, our design incorporates 72 points counters, 72 Range Scale First Index (RSFI) fip-fops, and 72 Range Scale Occupancy Count (RSOC) counters. The number 72 corresponds to the value of $M$ in our design. Collectively, these components facilitate the reordering of all 64 points within an RSSD, thereby ensuring effcient parallel processing.  

V. FAST AND CONFIGURABLE CORRESPONDENCE SEARCH  

In this section, we frst introduce the procedure of searching for the KNN-C, PNN-C, and ENN-C based on the RPS structure. Subsequently, we propose the hardware implementation of the RPS-Searcher accelerator.  

# A. Searching for Multi-type Correspondence based on RPS  

From Section II-A, PNN-C is identifed as the most complex correspondence type, as it encompasses the search processes of both KNN-C and ENN-C. Therefore, we use PNN-C as an example to illustrate the search process based on the RPS structure and search range 𝑟𝑖𝑛.  

For a query point $P_{\mathcal{H}}^{i}\,^{\,\,\,\overline{{\prime}}}$ , the PNN-C consists of points $(Q_{\mathcal{H}}^{j},$ $Q_{\mathcal{H}}^{m},\;Q_{\mathcal{H}}^{l})\,\in\,Q_{\mathcal{H}}$ . The search process involves fve steps, as shown in Fig. 9:  

1) Determining the RPS position of the query point: Compute the RPS position $(\nu_{i},h_{i},R_{i})$ of $P_{\mathcal{H}}^{i}$ , including the row index $\nu_{i}$ , column index $h_{i}$ , and range scale $R_{i}$ , as described in Section IV. For example, in Fig. 9 (ab), the query point (red star) has an RPS position of $(3,1,2)$ .  

![](images/93a71b7500894d792ca424f2c70304ea6be8151b2480a9bd2bde961c4e0446c2.jpg)  
Fig. 9. An example of searching for correspondence based on RPS. These fgures are modifed from [31].  

2) Calculating the search region: Defne the search region as a cube centered at $(\nu_{i},h_{i},r_{i})$ with a side length of $2r_{i n}$ . First, determine the search region dimensions $h_{s r}=$ $[h_{m i n},h_{m a x}]$ and $R_{s r}=[R_{m i n},R_{m a x}]$ using equation (4) and LUTs based on $r_{i}$ . For example, the search region for the red star is $h_{s r}\,=\,[0,~2]$ and $R_{s r}\,=\,[2,\ 3]$ , as shown in Fig. 9 (a-b). Second, map these dimensions onto RPS-Index array indices to defne search blocks $(S B=[S B_{m i n},S B_{m a x}))$ using equation (5). For instance, Fig. 9 (c) defnes search blocks as intervals [2, 4), [6, 9), and [10, 12).  

$$
\begin{array}{r l}&{h_{m i n}=h_{i}-a r c s i n{(r_{i n}/r_{i})}/{\Delta\alpha}}\\ &{h_{m a x}=h_{i}+a r c s i n{(r_{i n}/r_{i})}/{\Delta\alpha}}\\ &{R_{m i n}=R L U T[r_{i}-r_{i n}]}\\ &{R_{m a x}=R L U T[r_{i}+r_{i n}]}\end{array}
$$  

$$
\begin{array}{r l}&{S B_{m i n}=R P S I[h_{c}*M+R_{m i n}]}\\ &{S B_{m a x}=R P S I[h_{c}*M+R_{m a x}+1]}\end{array}
$$  

3) Extracting candidate corresponding points in the search region: Extract relevant points from the RPSPoints array based on the search blocks, designating them as candidate corresponding points. Fig. 9 (d) highlights points in the search blocks with red rectangles. The structured RPS-Points array enables parallel extraction of points within each block.  

4) Searching for the KNN-C in the candidate corresponding points: Within each search block, calculate the square distances from the query point to candidate points to select KNN points, which are streamed to subsequent steps as candidate geometric points. A priority queue [30] is used to search for KNN-C across all candidate points, identifying the nearest point as $Q_{\mathcal{H}}^{j}$ .  

5) Searching for PENN-C from the candidate geometric points. Unlike the priority queue K-selection method [30], we propose a fast conditional K-selection method to effciently select $Q_{\mathcal{H}}^{m}$ and $Q_{\mathcal{H}}^{l}$ using laser channel  

Algorithm 2 Lased-Channel Based Parallel Conditional Pri  
ority Queue K-selection Method for Streaming Batch Points   
Input: Candidate geometric points $\overline{{\operatorname{GP}[N_{g}]}}$ , with range . $.r$ , row index . $\boldsymbol{\cdot}\boldsymbol{\nu}$ , and rps index $\cdot i$ ; Nearest point $Q_{j}$   
Output: PNN-C: $Q_{l}$ and $Q_{m}$   
1: for $i\in[0,N_{g})$ parallel do ⊲ For All Points in GP   
2: gap of column index: $G_{\nu}[\mathrm{i}]\leftarrow\mathrm{abs}(\mathrm{GP[i].v~-~}Q_{j}.\mathrm{v})$   
3: gap of RPS-index: $G_{i}[\mathrm{i}]\leftarrow\mathrm{abs}(\mathrm{GP[i].i\leftarrow\partial_{j}.i}$ )   
4: set the compare array $C_{j}$ and $C_{m}$ to {-1}   
5: for $j\in[0,N_{g})$ parallel do $\blacktriangleright$ Conditional Compare   
6: $C_{m}[\mathrm{i}]\leftarrow$ compare i and j, $G_{\nu}[\mathrm{i}]$ and $G_{\nu}$ [j]   
7: $C_{j}[\mathrm{i}]\leftarrow$ further compare $G_{i}$ [i] and $G_{i}$ [j]   
8: end for   
9: if $C_{j}[\mathrm{i}]{=}0$ & GP[i]. $.\mathbf{r}{<}Q_{l}$ .r & $G_{\nu}[\mathrm{i}]{=}0$ & $G_{i}[\mathrm{i}]\!>\!0$ then   
10: $Q_{l}\leftarrow\mathrm{GP[i]}$   
11: end if   
12: if $C_{m}[\mathrm{i}]{=}0$ & GP[i]. $\mathbf{r}{<}Q_{m}$ .r & $0{<}G_{\nu}[\mathrm{i}]{<}3$ then   
13: $Q_{m}\leftarrow\mathrm{GP[i]}$   
14: end if   
15: end for  

information and the distance-ordered candidate points, as detailed in Algorithm 2.  

The methodology for PNN-C can be adapted for other correspondence types:  

• For KNN-C: Skip step 5 and directly output KNN correspondences in step 4.   
• For ENN-C: Modify step 5 to identify $Q_{\mathcal{E}}^{m}$ and divide the column index $h$ by 5 to account for the reduced size of the point cloud matrix in the specialized ENN-C RPS structure.  

# B. Hardware Implementation of RPS-Searcher  

This subsection outlines the hardware architecture and optimization strategies of the proposed RPS-Searcher accelerator, which enhances task-level pipeline performance while maintaining high search accuracy. Based on the search algorithm in Section V-A, the accelerator comprises seven modules: Transform-Points-Projection (TPP), Compute-SearchRegion (CSR), Extract-Region-Index (ERI), Extract-NearPoints (ENP), Points-Batcher (PB), KNN-CS, and PNN-CS, as shown in Fig. 6. Key optimizations for each module are detailed below.  

1) TPP Module: The TPP module transforms source points into the target coordinate system and projects query points into the RPS structure. Two primary optimizations are applied:  

• Bit-width Optimization: The bit-width of source points is optimized to 20 bits, ensuring suffcient accuracy while minimizing hardware resources.   
• Matrix Multiplication Unrolling: The matrix multiplication process is fully unrolled to accelerate computations and improve hardware effciency.  

2) CSR Module: The CSR module computes search regions for query points, represented by column indices. As illustrated in Fig. 10, a proximity-based search optimization strategy is employed, using a sequence generator $(0,-1,1,-2,2,\ldots)$ to prioritize nearby columns. Once suffcient points are found, searches in distant columns are skipped, improving effciency.  

![](images/28ef04b45ba6fce25b43942121898fb259f49d25ec638a2d5fb54d5f50b0055e.jpg)  
Fig. 10. Hardware Implementation of the CSR and ERI module.  

3) ERI Module: The ERI module extracts search blocks $(S B_{m i n},S B_{m a x})$ from search regions. There are two challenges when implementing the ERI module. (1) The capacity of onchip memory is insuffcient to store the entire RPS-Index array, and accessing external memory introduces signifcant latency. (2) Redundant points within the search region negatively impact performance. To address these challenges, as Fig. 10 shows, we present a dynamic caching strategy that incorporates the following three techniques:  

• Optimized on-chip caching: Observing that both the RPS-Index and input query points are roughly ordered in a RSSD-wise manner, we design an on-chip sliding window cache, referred to as the RPSI-Cache (Fig. 6). The RPSI-Cache has two key features: (1) It is compact, occupying only about $3.5\%$ of the size of the original RPS-Index array. This reduced size is suffcient to overlap with neighboring search regions for any query point, as confrmed by profling results. (2) The RPSI-Cache is dynamically refreshed based on the location of the query point, ensuring continuous coverage of adjacent regions. • Effcient cache refresh strategy: We refresh the outdated portion of the RPSI-Cache with new data when the column index of the query point exceeds the current reading column index by a predefned threshold. This strategy is crucial for maintaining the high-throughput pipeline structure of the ERI module. To further improve performance, the RPSI-Cache is cyclically partitioned into multiple segments and bound to 2-Port BRAM, enabling concurrent reading and writing of multiple RPSI values simultaneously.  

4) ENP Module: The ENP module is designed to extract candidate corresponding points from the RPS-Points array based on the identifed search blocks. To enhance effciency, we also adopt the dynamic caching strategy while further optimizing points-read parallelism to meet performance requirements. We pack 4 candidate points into a single large bit-width element in the RPSP-Cache array and partition the array into 16 independent segments, enabling the concurrent reading of all 48 candidate points in parallel.  

5) PB Module: The PB module aggregates a variable number of candidate points into fxed-size batches to enable effcient dense computation in the subsequent KNN-CS module.  

The motivation for this design lies in the highly variable number of candidate points for a single input, which can range from 1 to 48. Directly transmitting up to 48 points to the next stage would require padding most of the time, introducing ineffciencies such as wasted hardware resources, increased power consumption, and elongated clock paths, which degrade timing performance. At the same time, discarding excess points would compromise search accuracy, as critical data might be lost.  

To address this, we propose a dynamic batching strategy that adjusts the varying numbers of candidate points in the RSSD and packages them into fxed-size batches compatible with the requirements of the next module. By dynamically controlling data buffering and output, the PB module ensures that the search accuracy is preserved, while optimizing resource utilization and reducing overhead.  

Effcient buffering with a shift batch buffer is employed to implement this module in a pipeline architecture while minimizing hardware resource consumption. The buffer, sized at $256\!\times\!64$ , is controlled by a pointer that tracks the number of valid candidate points. Incoming data is stored at the location indicated by the pointer, while invalid points are overwritten by new valid points, ensuring effcient memory use. Once the pointer exceeds a predefned threshold, the module outputs a dense batch of points to the next stage and resets the pointer. This design not only ensures high throughput but also minimizes resource usage.  

6) KNN-CS Module: The KNN-CS module focuses on sorting candidate corresponding points based on their distance to the query point and selecting the K-Nearest Neighbors (KNN) to identify $Q_{\mathcal{H}}^{j}$ .  

Sorting and selecting KNN points from streaming candidate points is computationally expensive due to the large number of required comparisons and excessive data bandwidth consumption. A naive sorting approach would require a large comparator array, increasing hardware complexity and power consumption.  

To address this, we employ an optimized K-selection priority queue method, which effciently selects and sorts the KNN points while reducing the number of comparators and narrowing data bandwidth. As candidate points stream into the module, a parallel comparing network is used to select KNN points from the incoming data, signifcantly reducing the data bandwidth. These KNN points are then compared with the contents of a priority queue, which is also sized to K, to update the queue. Once all candidate points have been processed, the priority queue contains the fnal KNN correspondences.  

This approach leverages an effcient priority queue architecture introduced in our prior work [30], allowing for reduced hardware resource usage and lower power consumption, while simultaneously improving the performance of subsequent stages.  

7) PENN-CS Module: The PENN-CS module is responsible for selecting PNN-C or ENN-C from the candidate geometric points. The selection process is computationally intensive, as it typically requires large comparator arrays and resource-heavy in-order comparisons to fnd the desired points. Moreover, unordered geometric points make the selection process less effcient, increasing resource utilization and processing time.  

To optimize this process, we leverage laser channel information and the ordered nature of KNN points to improve effciency and reduce hardware requirements. The module employs two key strategies:  

• Effcient point selection using laser channel attributes: Unlike previous methods [30] that rely on large comparator arrays, we use the unique laser channel properties of points within a search block for direct selection. Points in a search block belong to the same column and have unique laser channel indices (row index), and the laser channel of $Q_{\mathcal{H}}^{m}$ matches that of $Q_{\mathcal{H}}^{j}$ . Based on this observation, we directly compare laser channels and distances to rapidly identify $Q_{\mathcal{H}}^{m}$ , as detailed in Algorithm 2. This signifcantly reduces the comparator array size while improving selection accuracy.   
• Resource-effcient compare logic optimization: Instead of selecting from unordered points, the PENN-CS module identifes $Q_{\mathcal{H}}^{m}$ from the already ordered KNN points. First, the most suitable point $\widehat{Q}_{\mathcal{H}}^{l}$ is selected from the input K candidate geometric po ints based on their laser channels and order indices. Then, $\widehat{Q}_{\mathcal{H}}^{l}$ is compared with the current $\boldsymbol{Q}_{\boldsymbol{H}}^{l},$ and the nearer poin t  is retained in $Q_{\mathcal{H}}^{l}$ . This reduces both the number and bit-width of required comparators, signifcantly lowering resource utilization and improving effciency.  

By combining laser channel-based selection and compare logic optimization, the PENN-CS module achieves a balance between performance and resource effciency, ensuring that geometric corresponding points are selected accurately with minimal hardware overhead.  

# VI. EXPERIMENTAL RESULTS AND ANALYSIS  

In this section, we frst describe the experimental setup, encompassing the test datasets, evaluation cases, parameters, various implementations, and testing platforms. Subsequently, we conduct a comparative analysis of runtime, power consumption, and resource utilization across different correspondence search implementations. Following this, we provide a detailed analysis of the proposed RPS-CS accelerator, including the latency and resource utilization of individual modules. Finally, we integrate the RPS-CS based registration framework into the L-SLAM system, replacing its existing registration algorithm. This integration enables us to evaluate the framework’s impact on accuracy and runtime performance in a practical application context.  

# A. Experimental Setup  

Experimental datasets: To evaluate the proposed point cloud RPS-CS accelerator and registration framework, we utilize the KITTI dataset [10], specifcally the Visual Odometry/SLAM Evaluation 2012 point cloud. This dataset includes diverse road environment data captured during real-world driving scenarios. For our evaluation, we focus exclusively on point clouds collected by the Velodyne HDL-64E LiDAR sensor, which employs 64 lasers and operates at a scanning frequency of $10\mathrm{Hz}$ .  

TABLE I EXPERIMENTAL TEST CASES.   


<html><body><table><tr><td>Cases</td><td>Case I</td><td>Case II</td><td>Case IⅢI</td><td colspan="2">Case IV</td></tr><tr><td>Type</td><td>All Types</td><td>All types</td><td>All t types</td><td>PNN-C</td><td>ENN-C</td></tr><tr><td>NQ</td><td>30,000</td><td>1,000 to 80,000</td><td>30,000</td><td>30,000</td><td>5000</td></tr><tr><td>Np</td><td>30,000</td><td>30,000</td><td>1,000 to 80,000</td><td>1,400</td><td>750</td></tr></table></body></html>  

Experimental Cases: To comprehensively evaluate the performance of the proposed RPS-CS accelerator, we design four representative experimental cases, as summarized in Table I. Case I adopts the standard settings from prior works [5], [14] and serves as a baseline for comparison. Case II and Case III assess the scalability and robustness of the accelerator under varying confgurations of $N_{P}$ and $N_{Q}$ . Finally, Case IV evaluates the real-world applicability of the RPS-CS accelerator by analyzing the average sizes of $N_{P}$ and $N_{Q}$ in a typical point cloud registration task, as outlined in [1].  

For all cases, the point clouds are downsampled from ten randomly selected consecutive frame point clouds from various scenarios within the KITTI dataset.  

Experimental platforms: ARM-based measurements were conducted on an Apple Silicon M4-equipped Mac Mini (4.4 GHz). FPGA implementations were developed using Xilinx Vitis 2022.2 and validated on a Xilinx UltraScale $^+$ MPSoC ZCU104 board $(200\ \mathrm{MHz})$ . Power measurements for FPGA platforms were acquired through an embedded INA226 sensor via the PMBus; ARM platform measurements utilized offcial power management library APIs. All measurements refect total platform power consumption to ensure equitable comparison.  

Experimental parameters: In alignment with the specifcations of the 64-laser LiDAR utilized in the KITTI dataset, we have meticulously confgured the experimental parameters. Specifcally, we establish the horizontal resolution $H=1800$ (360 for ENN-C) and the vertical resolution $V=64$ to facilitate the generation of the point cloud matrix. Besides, we set $N_{s d}\,=\,4$ , $M\,=\,72$ , and $r_{i n}\,=\,1$ . This ensures the accuracy of the correspondence search results is greater than $95\%$ of all test cases.  

# B. Comparison of Run-Time, Power, and Resources  

Table II summarizes the average run-time per frame for various correspondence search implementations on Test Case I. The FPGA-based PNN-CS Time is derived by combining a high-parallel $\boldsymbol{\mathrm{k}}$ -selection implementation [5] with the reported KNN-CS Time. Profling of the original PNN operation [1] shows that it searches an average of 2,338 neighboring points to select PNN correspondences for each query point. To address this, we developed a high-parallel $\boldsymbol{\mathrm{k}}$ -selection accelerator capable of processing 2,338 neighboring points for 30,000 queries in $21.9~\mathrm{ms}$ . As the k-selection accelerator operates in parallel with the original KNN-CS accelerator in a task-level pipeline, the FPGA-based PNN-CS Time is determined as the maximum of the KNN-CS Time and $21.9~\mathrm{ms}$ .  

![](images/e7aa1344db87d091548fd38d933d9678ad71d8953d247043653746ef75821d40.jpg)  
Fig. 11. Performance results of RPS-CS accelerator.  

The results demonstrate that the proposed RPS-CS accelerator achieves exceptional performance, requiring only $5.0~\mathrm{ms}$ to construct the structure and search PNN correspondences for 30,000 points. Compared to other implementations, our RPSCS accelerator is $13.6\times$ , $7.5\times_{\mathrm{i}}$ , and $4.7\times$ faster than QuickNN [14], ParallelNN [9], and RPS-KNN [31] on the FPGA platform, respectively. Moreover, the RPS-CS accelerator achieves the best performance in both data structure build time and KNN/PNN correspondence search, confrming the effciency of the RPS structure and the high parallelism of the RPSBuilder and RPS-Searcher accelerators.  

Table II also compares the average power and energy consumption of different implementations. Using FLANN-CPU as the baseline, the energy effciency ratio (EER) is defned as the ratio of an implementation’s energy consumption to the baseline energy. The RPS-CS accelerator is $20.7\times$ , $187\times$ , and $3.6\times$ more energy effcient than QuickNN [14], ParallelNN [9], and RPS-KNN [31], respectively. This energy effciency is largely attributed to dynamic voltage scaling [32], [33], where the supply voltage of the programmable logic (PL) side is dynamically scaled at a fxed ${200}\,\mathrm{MHz}$ frequency to minimize power consumption while maintaining correct results.  

Since the performance $(5.0\,\mathrm{\ms})$ far exceeds real-time requirements, we focus on area optimization to further improve energy effciency. Table III shows the fully routed resource utilization of the RPS-Builder, RPS-Searcher, and the total RPS-CS accelerator. Compared to QuickNN, RPS-CS uses fewer reconfgurable resources (LUTs and FFs) and DSPs but requires more memory to store the RPS-Points and RPS-Index array. Similarly, RPS-CS uses fewer hardware resources than DSVS, except for LUTs.  

# C. Detail Analysis of RPS-CS Accelerator  

To comprehensively analyze the proposed RPS-CS accelerator, we evaluate its performance on Test Cases II and III. Fig. 11 shows the run-time during RPS structure construction and correspondence searches.  

Experimental results reveal that the RPS structure’s build time is consistently fxed at $1.4~\mathrm{ms}$ for KNN/PNN correspondences and $0.4~\mathrm{ms}$ for SNN correspondences. This consistency is due to the fxed-size point cloud matrix organization (with $V=64$ and $H=1800$ for KNN/PNN, and $V=64$ and $H=450$ for SNN). The builder module processes all operations based on this fxed size, ensuring constant build time.  

TABLE II COMPARISON OF DIFFERENT KNN/PNN CORRESPONDENCE SEARCH IMPLEMENTATIONS   


<html><body><table><tr><td>Implementations</td><td>TPAMI'14 (FLANN) [4]</td><td>Trans.IE'20 (Graph) [15]</td><td>HPCA'20 (QuickNN) [14]</td><td>TCAS-II'20 (DSVS)[5]</td><td>HPCA'23 (ParallelNN) [9]</td><td>ISCAS'23 (RPS-KNN) [31]</td><td>This Work (RPS-CS)</td></tr><tr><td>Hardware</td><td>CPU (Mac Mini M4)</td><td>FPGA (ZCU102)</td><td>FPGA (VCU118)</td><td>FPGA (ZCU102)</td><td>FPGA (VCU128)</td><td>FPGA (ZCU104)</td><td>FPGA (ZCU104)</td></tr><tr><td>Process</td><td>3nm</td><td>16nm</td><td>16nm</td><td>16nm</td><td>16nm</td><td>16nm</td><td>16nm</td></tr><tr><td>SearchStructure</td><td>K-D Tree</td><td>Graph</td><td>K-D Tree</td><td>DSVS</td><td>Octree (1024)</td><td>RPS</td><td>RPS</td></tr><tr><td>Build Time (ms)</td><td>3.4</td><td>289.0</td><td>46.0</td><td>4.8</td><td>0.5</td><td>1.5</td><td>1.4</td></tr><tr><td>KNN-CS Time (ms)</td><td>9.0</td><td>146.0</td><td>10.5</td><td>69.0</td><td>2.0</td><td>2.6</td><td>2.8</td></tr><tr><td>PNN-CS Time (ms)</td><td>110.1</td><td>146.0</td><td>21.9</td><td>69.0</td><td>21.9</td><td>21.9</td><td>3.6</td></tr><tr><td>Total Time (ms) +</td><td>113.5</td><td>435.0</td><td>67.9</td><td>73.8</td><td>22.4</td><td>23.4</td><td>5.0</td></tr><tr><td>Speedup Ratio +</td><td>1.0</td><td>0.3</td><td>1.7</td><td>1.5</td><td>3.0</td><td>4.9</td><td>22.7</td></tr><tr><td>Average Power (W)</td><td>12.9</td><td>4.2</td><td>4.7</td><td>3.6</td><td>28.4</td><td>2.4</td><td>3.1</td></tr><tr><td>Energy (mJ)</td><td>1464.2</td><td>1827.0</td><td>321.2</td><td>265.7</td><td>636.2</td><td>55.2</td><td>15.5</td></tr><tr><td>EER +</td><td>1.0</td><td>0.8</td><td>4.6</td><td>5.5</td><td>0.5</td><td>26.5</td><td>94.5</td></tr></table></body></html>

\* PNN-CS time is based on a high-parallel $\mathbf{k}$ -selection implementation [5] and the reported KNN-CS time. + Total Time is the sum of Build Time and PNN-CS Time. Speedup Ratio is the Total Time relative to the baseline. EER (Energy-Effciency Ratio) is the Energy relative to the baseline.  

TABLE III RESOURCE UTILIZATION COMPARISON OF DIFFERENT CORRESPONDENCE SEARCH ACCELERATORS   


<html><body><table><tr><td colspan="2">Implmentations</td><td>LUTs</td><td>FFs</td><td>BRAM36</td><td>DSP</td></tr><tr><td>QuickNN [14]</td><td>Total</td><td>203,758</td><td>152,962</td><td>1</td><td>896</td></tr><tr><td>DSVS [5]</td><td>Search</td><td>68,960</td><td>65,024</td><td>350</td><td>83</td></tr><tr><td>ParalleINN [9]</td><td>Search</td><td>141,132</td><td>177,112</td><td>116</td><td>480</td></tr><tr><td rowspan="3">RPS-CS (ours)</td><td>Build</td><td>56,733</td><td>43,372</td><td>105</td><td>19</td></tr><tr><td>Search</td><td>93,149</td><td>52,465</td><td>251</td><td>43</td></tr><tr><td>Total</td><td>153,722</td><td>143,765</td><td>362.5</td><td>62</td></tr></table></body></html>  

The search time shows a nearly linear relationship with the number of source points, indicating that the performance bottleneck in the RPS cache [31] has been resolved. This improvement stems from expanding the cache port bit-width and optimizing the cache strategy to dynamically adapt to query requirements. However, when the number of queries is below 3,000, the search time for PNN-C and KNN-C remains constant due to the dominance of data transfer for the large target point volume. Beyond this threshold, search time becomes the primary factor, growing linearly with the number of queries.  

Additionally, the slope of this linear growth is determined by the preset maximum search range. Since KNN-C and ENNC share the same range, their slopes are nearly identical. In contrast, PNN-C requires a larger search range to maintain an accuracy above $95\%$ , resulting in a steeper slope and faster search time growth as the number of queries increases.  

# D. Evaluation of Registration Framework  

To validate the RPS-CS accelerator within the registration framework, we evaluate its run-time performance on Test Case IV and assess its impact on accuracy and speed when integrated into a SLAM system.  

TABLE IV ACCURACY COMPARISON TABLE OF DIFFERENT REGISTRATION IMPLEMENTATIONS   


<html><body><table><tr><td rowspan="2">Seq.</td><td rowspan="2">Enviroment</td><td colspan="2">LOAM</td><td colspan="2">LOAM-RPS</td><td colspan="2">LOAM-RPS-HW</td></tr><tr><td>trel</td><td>rrel</td><td>trel</td><td>rrel</td><td>trel</td><td>rrel</td></tr><tr><td>00</td><td>Urban</td><td>1.0340</td><td>0.0046</td><td>1.0369</td><td>0.0047</td><td>1.0844</td><td>0.0047</td></tr><tr><td>01</td><td>Highway</td><td>2.8464</td><td>0.0060</td><td>2.8332</td><td>0.0060</td><td>2.8357</td><td>0.0060</td></tr><tr><td>02</td><td>Urban+Country</td><td>3.0655</td><td>0.0114</td><td>3.1339</td><td>0.0114</td><td>3.2206</td><td>0.0117</td></tr><tr><td>03</td><td>Country</td><td>1.0822</td><td>0.0066</td><td>1.0858</td><td>0.0066</td><td>1.0884</td><td>0.0066</td></tr><tr><td>04</td><td>Country</td><td>1.5248</td><td>0.0055</td><td>1.5336</td><td>0.0054</td><td>1.5364</td><td>0.0054</td></tr><tr><td>05</td><td>Urban</td><td>0.7184</td><td>0.0036</td><td>0.7520</td><td>0.0037</td><td>0.7705</td><td>0.0038</td></tr><tr><td>06</td><td>Urban</td><td>0.7627</td><td>0.0040</td><td>0.7781</td><td>0.0040</td><td>0.7775</td><td>0.0041</td></tr><tr><td>07</td><td>Urban</td><td>0.5629</td><td>0.0040</td><td>0.5719</td><td>0.0038</td><td>0.5750</td><td>0.0038</td></tr><tr><td>08</td><td>Urban+Country</td><td>1.2320</td><td>0.0052</td><td>1.1680</td><td>0.0048</td><td>1.1659</td><td>0.0049</td></tr><tr><td>09</td><td>Urban+Country</td><td>1.3103</td><td>0.0053</td><td>1.3062</td><td>0.0052</td><td>1.3041</td><td>0.0053</td></tr><tr><td>10</td><td>Urban+Country</td><td>1.6843</td><td>0.0059</td><td>1.6317</td><td>0.0059</td><td>1.6211</td><td>0.0060</td></tr><tr><td></td><td>MeanError</td><td>1.4385</td><td>0.0056</td><td>1.4392</td><td>0.0056</td><td>1.4527</td><td>0.0057</td></tr></table></body></html>

$t_{r e l}$ : the average translational RMSE $(\%)$ on length of $100\mathrm{m}{\cdot}800\mathrm{m}$ . $r_{r e l}$ : the average rotational RMSE $^{\circ}/100\mathrm{m}$ on length of $100\mathrm{m}{\cdot}800\mathrm{m}$  

On Case IV, the RPS-CS accelerator achieves run-times consistent with Fig. 11. For SNN-C, it requires $0.4~\mathrm{ms}$ to build the RPS structure and $0.6~\mathrm{ms}$ to search for correspondences. For PNN and KNN correspondences, the times are $1.4~\mathrm{ms}$ for building and $1.9~\mathrm{ms}$ for searching.  

To evaluate accuracy, the RPS-CS accelerator is integrated into a SLAM system, and localization trajectory accuracy is assessed via the following implementations:  

• LOAM: The original LOAM [1], implemented on the FPGA PS side, serves as the baseline. • LOAM-RPS: LOAM with foating-point RPS-CS integration on the FPGA PS side. • LOAM-RPS-HW: LOAM with fxed-point RPS-based registration framework on the FPGA PL side.  

The accuracy results for the three implementations are presented in Table IV, evaluated using the offcial KITTI evaluation tool. It can be concluded that our proposed RPS-based registration framework has a此 数ne据gl来ig自ib腾le讯 i文m档p-a>cretg iostnra tiaocnc实ur验ac数y, with an observed decrease 需of要 a搜pp索r两ox次im，a所te以ly需 $1\%$ . 有In时 间ce乘rt2ain scenarios, our framework even achieves higher accuracy due to the reduction of outlier correspondence pairs.  

<html><body><table><tr><td colspan="5">LOAM</td></tr><tr><td>75.6</td><td></td><td>142</td><td>246.6</td><td>42.8</td></tr><tr><td>LOAM-RPS</td><td></td><td></td><td></td><td>507ms</td></tr><tr><td>68.5</td><td>51</td><td>93.5</td><td>42.8 255.8ms</td><td></td></tr><tr><td>LOAM-RPS-HW</td><td></td><td></td><td></td><td></td></tr><tr><td>42.8</td><td>49.6ms</td><td></td><td></td><td></td></tr><tr><td></td><td></td><td></td><td></td><td></td></tr><tr><td>1.8 1.2 3.8</td><td></td><td></td><td></td><td></td></tr></table></body></html>  

Fig. 12 illustrates the average registration time on the KITTI datasets. Two iterations of correspondence search in the registration task result in SNN and PNN search times of $1.2~\mathrm{ms}$ and $3.8\,\mathrm{~ms~}$ , respectively. The RPS-based registration algorithm achieves a $2\times$ improvement in overall speed, with correspondence search time enhanced by $2.7\times$ . Using the proposed framework, the overall running speed improves by $10.2\times$ , reaching 20.1 FPS, while correspondence searches are accelerated by $68.2\times$ .  

# VII. CONCLUSION  

In this work, we propose a real-time FPGA-based point cloud registration framework with ultra-fast and confgurable correspondence search capabilities. First, we develop a novel Range-Projection Structure (RPS) that organizes unstructured LiDAR points into a matrix-like format for effcient points localization, and grouping nearby points into continuous memory segments to accelerate points access. Second, we introduce an effcient multi-mode correspondence searching algorithm that leverages the RPS structure to effectively narrow the search region, eliminate redundant points, and support various types of correspondences by utilizing LiDAR-specifc laser channel information. Third, we design a confgurable ultrafast RPS-based correspondence search (RPS-CS) accelerator featuring a high-performance RPS-Builder for rapid structure construction and a highly parallel RPS-Searcher for fast correspondence searching, further enhanced by a dynamic caching strategy and a pipeline batcher module to improve effciency and confgurability. Experimental results show that the RPS-CS accelerator achieves a $7.5\times$ speedup and $17.1\times$ energy effciency improvement over state-of-the-art FPGA implementations, while the proposed framework achieves realtime performance for 64-laser LiDAR data with a frame rate of 20.1 FPS.  

In future work, we will aim to overcome bandwidth limitations for large-scale parallelization and extend the proposed approach to accommodate general point clouds.  

# REFERENCES  

[1] J. Zhang and S. Singh, “Low-drift and real-time lidar odometry and mapping,” Autonomous robots, vol. 41, no. 2, pp. 401–416, Feb 2017.   
[2] X. Zhang and X. Huang, “Real-time fast channel clustering for lidar point cloud,” IEEE Transactions on Circuits and Systems II: Express Briefs, vol. 69, no. 10, pp. 4103–4107, Oct 2022.   
[3] C.-C. Wang, Y.-C. Ding, C.-T. Chiu, C.-T. Huang, Y.-Y. Cheng, S.-Y. Sun, C.-H. Cheng, and H.-K. Kuo, “Real-time block-based embedded cnn for gesture classifcation on an fpga,” IEEE Transactions on Circuits and Systems I: Regular Papers, vol. 68, no. 10, pp. 4182–4193, Oct 2021.   
[4] M. Muja and D. G. Lowe, “Scalable nearest neighbor algorithms for high dimensional data,” IEEE Transactions on Pattern Analysis and Machine Intelligence, vol. 36, no. 11, pp. 2227–2240, Nov 2014.   
[5] H. Sun, X. Liu, Q. Deng, W. Jiang, S. Luo, and Y. Ha, “Effcient fpga implementation of k-nearest-neighbor search algorithm for 3d lidar localization and mapping in smart vehicles,” IEEE Transactions on Circuits and Systems II: Express Briefs, vol. 67, no. 9, pp. 1644–1648, Sep. 2020.   
[6] Q. Deng, H. Sun, F. Chen, Y. Shu, H. Wang, and Y. Ha, “An optimized f评pg估a-结bas果ed real-time ndt for 3d-lidar localization in smart vehicles,” IEEE Transactions on Circuits and Systems II: Express Briefs, vol. 68, no. 9, pp. 3167–3171, Sep. 2021.   
[7] E. Bank Tavakoli, A. Beygi, and X. Yao, “Rpknn: An opencl-based fpga implementation of the dimensionality-reduced knn algorithm using random projection,” IEEE Transactions on Very Large Scale Integration (VLSI) Systems, vol. 30, no. 4, pp. 549–552, April 2022.   
[8] C. Wang, Z. Huang, A. Ren, and X. Zhang, “An fpga-based knn seach accelerator for point cloud registration,” in 2024 IEEE International Symposium on Circuits and Systems (ISCAS), May 2024, pp. 1–5.   
[9] F. Chen, R. Ying, J. Xue, F. Wen, and P. Liu, “Parallelnn: A parallel octree-based nearest neighbor search accelerator for 3d point clouds,” in 2023 IEEE International Symposium on High-Performance Computer Architecture (HPCA), Feb 2023, pp. 403–414.   
[10] M. Han, L. Wang, L. Xiao, H. Zhang, T. Cai, J. Xu, Y. Wu, C. Zhang, and X. Xu, “Bitnn: A bit-serial accelerator for k-nearest neighbor search in point clouds,” in 2024 ACM/IEEE 51st Annual International Symposium on Computer Architecture (ISCA), June 2024, pp. 1278–1292.   
[11] F. Groh, L. Ruppert, P. Wieschollek, and H. P. A. Lensch, “Ggnn: Graphbased gpu nearest neighbor search,” IEEE Transactions on Big Data, vol. 9, no. 1, pp. 267–279, Feb 2023.   
[12] K. Koide, M. Yokozuka, S. Oishi, and A. Banno, “Voxelized gicp for fast and accurate 3d point cloud registration,” in 2021 IEEE International Conference on Robotics and Automation (ICRA), May 2021, pp. 11 054– 11 059.   
[13] W. Dong, J. Park, Y. Yang, and M. Kaess, “Gpu accelerated robust scene reconstruction,” in 2019 IEEE/RSJ International Conference on Intelligent Robots and Systems (IROS), Nov 2019, pp. 7863–7870.   
[14] R. Pinkham, S. Zeng, and Z. Zhang, “Quicknn: Memory and performance optimization of k-d tree based nearest neighbor search for 3d point clouds,” in 2020 IEEE International Symposium on High Performance Computer Architecture (HPCA), Feb 2020, pp. 180–192.   
[15] A. Kosuge, K. Yamamoto, Y. Akamine, and T. Oshima, “An soc-fpgabased iterative-closest-point accelerator enabling faster picking robots,” IEEE Transactions on Industrial Electronics, vol. 68, no. 4, pp. 3567– 3576, April 2021.   
[16] T. Shan and B. Englot, “Lego-loam: Lightweight and ground-optimized lidar odometry and mapping on variable terrain,” in 2018 IEEE/RSJ International Conference on Intelligent Robots and Systems (IROS), Oct 2018, pp. 4758–4765.   
[17] H. Wang, C. Wang, C.-L. Chen, and L. Xie, “F-loam: Fast lidar odometry and mapping,” in 2021 IEEE/RSJ International Conference on Intelligent Robots and Systems (IROS), Sep. 2021, pp. 4390–4396.   
[18] Y. Zhang, Z. Zhou, P. David, X. Yue, Z. Xi, B. Gong, and H. Foroosh, “Polarnet: An improved grid representation for online lidar point clouds semantic segmentation,” in Proceedings of the IEEE/CVF Conference on Computer Vision and Pattern Recognition (CVPR), June 2020, pp. 9601–9610.   
[19] R. Sun, J. Qian, R. H. Jose, Z. Gong, R. Miao, W. Xue, and P. Liu, “A fexible and effcient real-time orb-based full-hd image feature extraction accelerator,” IEEE Transactions on Very Large Scale Integration (VLSI) Systems, vol. 28, no. 2, pp. 565–575, Feb 2020.   
[20] R. Liu, J. Yang, Y. Chen, and W. Zhao, “eslam: An energy-effcient accelerator for real-time orb-slam on fpga platform,” in 2019 56th ACM/IEEE Design Automation Conference (DAC), June 2019, pp. 1– 6.   
[21] H. Yang, J. Shi, and L. Carlone, “Teaser: Fast and certifable point cloud registration,” IEEE Transactions on Robotics, vol. 37, no. 2, pp. 314– 333, April 2021.   
[22] F. Ma, G. V. Cavalheiro, and S. Karaman, “Self-supervised sparseto-dense: Self-supervised depth completion from lidar and monocular camera,” in 2019 International Conference on Robotics and Automation (ICRA), May 2019, pp. 3288–3295.   
[23] Y. Lyu, L. Bai, and X. Huang, “Chipnet: Real-time lidar processing for drivable region segmentation on an fpga,” IEEE Transactions on Circuits and Systems I: Regular Papers, vol. 66, no. 5, pp. 1769–1779, May 2019.   
[24] Y. Liu, J. Li, K. Huang, X. Li, X. Qi, L. Chang, Y. Long, and J. Zhou, “Mobilesp: An fpga-based real-time keypoint extraction hardware accelerator for mobile vslam,” IEEE Transactions on Circuits and Systems I: Regular Papers, vol. 69, no. 12, pp. 4919–4929, Dec 2022.   
[25] X. Zhang, L. Zhang, and X. Lou, “A raw image-based end-to-end object detection accelerator using hog features,” IEEE Transactions on Circuits and Systems I: Regular Papers, vol. 69, no. 1, pp. 322–333, Jan 2022.   
[26] Y. Li, M. Li, C. Chen, X. Zou, H. Shao, F. Tang, and K. Li, “Simdiff: Point cloud acceleration by utilizing spatial similarity and differential execution,” IEEE Transactions on Computer-Aided Design of Integrated Circuits and Systems, vol. 44, no. 2, pp. 568–581, Feb 2025.   
[27] Y. Gao, C. Jiang, W. Piard, X. Chen, B. Patel, and H. Lam, “Hgpcn: A heterogeneous architecture for e2e embedded point cloud inference,” in 2024 57th IEEE/ACM International Symposium on Microarchitecture (MICRO), Nov 2024, pp. 1588–1600.   
[28] G. Yan, X. Liu, F. Chen, H. Wang, and Y. Ha, “Ultra-fast fpga implementation of graph cut algorithm with ripple push and early termination,” IEEE Transactions on Circuits and Systems I: Regular Papers, vol. 69, no. 4, pp. 1532–1545, April 2022.   
[29] C. Chen, X. Zou, H. Shao, Y. Li, and K. Li, “Point cloud acceleration by exploiting geometric similarity,” in 2023 56th IEEE/ACM International Symposium on Microarchitecture (MICRO), Dec 2023, pp. 1135–1147.   
[30] H. Sun, Q. Deng, X. Liu, Y. Shu, and Y. Ha, “An energy-effcient streambased fpga implementation of feature extraction algorithm for lidar point clouds with effective local-search,” IEEE Transactions on Circuits and Systems I: Regular Papers, vol. 70, no. 1, pp. 253–265, Jan 2023.   
[31] J. Xiao, H. Sun, Q. Deng, X. Liu, H. Zhang, C. He, Y. Shu, and Y. Ha, “Rps-knn: An ultra-fast fpga accelerator of range-projection-structure knearest-neighbor search for lidar odometry in smart vehicles,” in 2023 IEEE International Symposium on Circuits and Systems (ISCAS), May 2023, pp. 1–5.   
[32] F. Chen, H. Yu, W. Jiang, and Y. Ha, “Quality optimization of adaptive applications via deep reinforcement learning in energy harvesting edge devices,” IEEE Transactions on Computer-Aided Design of Integrated Circuits and Systems, vol. 41, no. 11, pp. 4873–4886, Nov 2022.   
[33] W. Jiang, H. Yu, H. Zhang, Y. Shu, R. Li, J. Chen, and Y. Ha, “Fodm: A framework for accurate online delay measurement supporting all timing paths in fpga,” IEEE Transactions on Very Large Scale Integration (VLSI) Systems, vol. 30, no. 4, pp. 502–514, April 2022.  

![](images/114adf66878a944931bb625d0ee0e6d43293f9573dbb5fb05e9ddf211e0d5c2c.jpg)  

Qi Deng received the B.S.degree in electronic and information engineering from ShanghaiTech University in 2018. He is currently pursuing the Ph.D degree with ShanghaiTech University; the Shanghai Advanced Research Institute Chinese Academy of Sciences: and the University of Chinese Academy of Sciences. His research interests include localization and perception algorithms in smart vehicles and its hardware acceleration.  

![](images/b6c2bb377630983a6da333dee552abf6bf2c0183fef628ffe3a9b0f9b343e422.jpg)  

Hao Sun (S’20–M’23) received the B.S. degree from Southeast University in 2018. He received the Ph.D. degree from the Shanghai Institute of Microsystem and Information Technology, Chinese Academy of Sciences, Shanghai, China, and the ShanghaiTech University, Reconfgurable and Intelligent Computing Lab, Shanghai in 2023. He is currently a Lecturer at Southeast University, China. His current research interests are centered around custom computing, hardware acceleration, LiDAR based localization and mapping.  

Jianzhong Xiao received the B.S. degree in school of automation from Nanjing University of Aeronautics and Astronautics in 2020. He is currently working toward the Ph.D. degree at ShanghaiTech University, Shanghai, China. His current research interests include hardware acceleration, ultralow power VLSI designs and localization.  

![](images/f844cd5ac55c56f0cdff24eab058b266aa0a8d2da4d47fc4f6136181784bef4f.jpg)  

![](images/98455084f18086a0f53a17ee238725a884f630bbf6bc01a116db19b012905bfe.jpg)  

Yuhao Shu (S’21) received the B.S. degree in electronic science and technology from Hefei University of Technology, Hefei, China, in 2019. He received the Ph.D. degree in electronic science and technology from ShanghaiTech University, Shanghai, China, in 2025. Currently, he is working as an associate professor at Nanjing University of Aeronautics and Astronautics, Nanjing, China. His current research interests include embedded memory design, in-memory computing, cryogenic CMOS circuits, and ultra-low power VLSI design.  

![](images/2ccf403b0c4e61cc006bea4c1020c8a4661755602ca3a99f548464c7c6ebbd18.jpg)  

Weixiong Jiang received the B.S. degree from Harbin Institute of Technology, Harbin, China, in 2017. He received the Ph.D. degree with the Shanghai Institute of Microsystem and Information Technology, Chinese Academy of Sciences, Shanghai, China, and the ShanghaiTech University, Reconfgurable and Intelligent Computing Lab, Shanghai in 2022. His current research interests include energyeffcient DNN acceleration as well as online slack measurement on FPGA.  

Hui Wang received the Ph.D. degree in Physics from the Institute of Semiconductors, Chinese Academy of Sciences, Beijing, China, in 2001. He is a full professor in Microelectronics, the Shanghai Advanced Research Institute, Chinese Academy of Sciences. His research interests include highperformance imaging and display panel driving.  

![](images/98d3875b42db1133cc2cf49ea74ada3688fbfc92b5077f04d696130293f645dc.jpg)  

![](images/b0d31b674bdf5953389fd09a4b44db8c6e898929a82b8e125f0ab8ee6508c22a.jpg)  

Yajun Ha (S’98–M’04–SM’09) received the B.S. degree from Zhejiang University, Hangzhou, China, in 1996, the M.Eng. degree from the National University of Singapore, Singapore, in 1999, and the Ph.D. degree from Katholieke Universiteit Leuven, Leuven, Belgium, in 2004, all in electrical engineering.  

He is currently a Professor at ShanghaiTech University, China. Before this, he was a Scientist and Director, I2R-BYD Joint Lab at Institute for Infocomm Research, Singapore, and an Adjunct Associate Professor at the Department of Electrical & Computer Engineering, National University of Singapore. Prior to this, he was an Assistant Professor with National University of Singapore.  

His research interests include reconfgurable computing, ultra-low power digital circuits and systems, embedded system architecture and design tools for applications in robots, smart vehicles and intelligent systems. He has published around 150 internationally peer-reviewed journal/conference papers on these topics.  

He has served a number of positions in the professional communities. He serves as the Editor-in-Chief for the IEEE Trans. on Circuits and Systems II: Express Briefs (2022-2023), the Associate Editor-in-Chief for the IEEE Trans. on Circuits and Systems II: Express Briefs (2020–2021), the Associate Editor for the IEEE Trans. on Circuits and Systems I: Regular Papers (2016–2019), the Associate Editor for the IEEE Trans. on Circuits and Systems II: Express Briefs (2011–2013), the Associate Editor for the IEEE Trans. on Very Large Scale Integration (VLSI) Systems (2013–2014), and the Journal of Low Power Electronics (since 2009). He has served as the TPC Co-Chair of ISICAS 2020, the General Co-Chair of ASP-DAC 2014; Program Co-Chair for FPT 2010 and FPT 2013; Chair of the Singapore Chapter of the IEEE Circuits and Systems (CAS) Society (2011 and 2012); Member of ASP-DAC Steering Committee; and Member of IEEE CAS VLSI and Applications Technical Committee. He has been the Program Committee Member for a number of well-known conferences in the felds of FPGAs and design tools, such as DAC, DATE, ASP-DAC, FPGA, FPL and FPT. He is the recipient of two IEEE/ACM Best Paper Awards. He is a senior member of IEEE.  